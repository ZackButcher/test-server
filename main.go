// Copyright 2017 Zack Butcher.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	petname "github.com/dustinkirkland/golang-petname"
	"github.com/spf13/cobra"
	text "github.com/tonnerre/golang-text"
)

const (
	defaultPort uint16 = 9000
	healthPath         = "/health"
	echoPath           = "/echo"
	livePath           = "/live"
	callPath           = "/call"
)

type config struct {
	servingPort, healthCheckPort, livenessPort uint16
	healthy                                    bool
	livenessDelay                              time.Duration
	id                                         string
}

func main() {
	cfg := &config{}

	root := &cobra.Command{
		Use:   "server",
		Short: "Starts Mixer as a server",
		Run: func(cmd *cobra.Command, args []string) {
			if cfg.id == "" {
				rand.Seed(time.Now().UTC().UnixNano())
				cfg.id = petname.Generate(2, "-")
				log.Printf("no ID provided at startup, picking a random one")
			}
			log.Printf("starting with ID: %v", cfg.id)
			log.SetPrefix(fmt.Sprintf("%s ", cfg.id))

			servers := map[uint16]*http.ServeMux{
				cfg.servingPort:     http.NewServeMux(),
				cfg.healthCheckPort: http.NewServeMux(),
				cfg.livenessPort:    http.NewServeMux(),
			}

			servers[cfg.servingPort].HandleFunc(echoPath, cfg.echo)
			servers[cfg.servingPort].HandleFunc(callPath, cfg.call)
			servers[cfg.healthCheckPort].HandleFunc(healthPath, cfg.health(cfg.healthy))
			servers[cfg.livenessPort].HandleFunc(livePath, cfg.live(cfg.livenessDelay))
			// For each of the servers, wire up a default handler so we can respond to any URL
			for _, server := range servers {
				server.HandleFunc("/", cfg.catchall)
			}

			log.Printf("listening for:\n/echo:     %d\n/health:   %d\n/liveness: %d\n", cfg.servingPort, cfg.healthCheckPort, cfg.livenessPort)

			wg := sync.WaitGroup{}

			for port, server := range servers {
				wg.Add(1)

				s := server
				p := port
				go func() {
					log.Printf("Starting listener on port %d\n", p)
					err := http.ListenAndServe(toAddress(p), s)
					log.Printf("%v\n", err)
					wg.Done()
				}()
			}

			wg.Wait()
		},
	}

	root.PersistentFlags().Uint16VarP(&cfg.servingPort, "server-port", "s", defaultPort, "Main port to serve on; always on /echo")
	root.PersistentFlags().Uint16VarP(&cfg.healthCheckPort, "health-port", "c", defaultPort, "Port to serve health checks on; always on /health")
	root.PersistentFlags().Uint16VarP(&cfg.livenessPort, "liveness-port", "l", defaultPort, "Port to serve liveness checks on; always on /live")
	root.PersistentFlags().BoolVar(&cfg.healthy, "healthy", true, "If false, the health check will report unhealthy")
	root.PersistentFlags().DurationVar(&cfg.livenessDelay, "liveness-delay", time.Second, "Delay before the server reports being alive")
	root.PersistentFlags().StringVar(&cfg.id, "id", "", "Name that identifies this instance (the ID is returned as part of the response)")

	if err := root.Execute(); err != nil {
		log.Printf("%v\n", err)
		os.Exit(-1)
	}
}

func (cfg config) live(delay time.Duration) func(w http.ResponseWriter, r *http.Request) {
	live := time.Now().Add(delay)
	log.Printf("will be live at %v given delay %v\n", live, delay)
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("got liveness request with headers:     %v\n", r.Header)
		if time.Now().After(live) {
			fmt.Fprintf(w, "%s - live", cfg.id)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}
}

func (cfg config) health(healthy bool) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("got health check request with headers: %v\n", r.Header)
		if healthy {
			fmt.Fprintf(w, "%s - healthy", cfg.id)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}
}

func (cfg config) echo(w http.ResponseWriter, r *http.Request) {
	log.Printf("got echo request with headers:         %v\n", r.Header)
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("got err reading body: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
	fmt.Fprintf(w, "%s echoing: %s", cfg.id, body)
}

func (cfg config) call(w http.ResponseWriter, r *http.Request) {
	log.Printf("got call request with URL %q and headers: %v", r.URL, r.Header)

	target := ""
	targets, ok := r.URL.Query()["target"]
	if ok && len(targets) > 0 && targets[0] != "" {
		target = targets[0]
		log.Printf("Found target in URI, using %q", target)
	} else {
		t, err := ioutil.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			log.Printf("got err reading body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		target = string(t)
		log.Printf("Found target in request body, using %q", target)
	}
	target = strings.TrimSpace(target)
	if len(target) == 0 {
		log.Printf("empty target, aborting call")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "No target to call - use the ?target query parameter or pass a URL as the request body.")
		return
	}

	if !strings.HasPrefix(target, "http") {
		log.Print("Prefixing target with 'http://'")
		target = "http://" + target
	}

	log.Printf("got call target: %q", target)
	resp, err := http.Get(target)
	if err != nil {
		fmt.Fprintf(w, "%s GET %q failed: %v", cfg.id, target, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	b, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	body := string(b)
	if err != nil {
		body = fmt.Sprintf("could not reading body: %v", err)
	}

	log.Printf("GET %q succeeded with response code %v", target, resp.StatusCode)

	fmt.Fprintf(w, `Server:
	%s
Called:
	%s
Response Status Code:
	%v
Response Body:
%s`, cfg.id, target, resp.StatusCode, text.Indent(body, "\t"))

	//fmt.Fprintf(w, "Server:\n\t%s\nCalled:\n\t%s\nStatus Code:\n\t%v\nResponse:\n%s", cfg.id, target, resp.StatusCode, text.Indent(body, "\t"))
	//fmt.Fprintf(w, "%s got response: %v\nwith body: %s", cfg.id, resp, body)
}

func (cfg config) catchall(w http.ResponseWriter, r *http.Request) {
	log.Printf("got catch-all request with headers:    %v\n", r.Header)
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
	fmt.Fprintf(w, "%s default handler echoing: %q", cfg.id, body)
}

func toAddress(port uint16) string {
	return fmt.Sprintf(":%d", port)
}
