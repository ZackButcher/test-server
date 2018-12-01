## Copyright 2017 Zack Butcher.
##
## Licensed under the Apache License, Version 2.0 (the "License");
## you may not use this file except in compliance with the License.
## You may obtain a copy of the License at
##
##     http://www.apache.org/licenses/LICENSE-2.0
##
## Unless required by applicable law or agreed to in writing, software
## distributed under the License is distributed on an "AS IS" BASIS,
## WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
## See the License for the specific language governing permissions and
## limitations under the License.

HUB := zackbutcher
TAG := v0.1

ISTIO_HUB := gcr.io/google.com/zbutcher-test
ISTIO_TAG := 6e646bb0accd3a7b3beac52f2bd402d39f861108

SHELL := /bin/zsh
ISTIO_DIR := ./istio-0.8.0

default: build

##### Go

build:
	go build .

test-server.linux:
	GOOS=linux go build -a --ldflags '-extldflags "-static"' -tags netgo -installsuffix netgo -o test-server .

##### Docker

docker.build: test-server.linux
	docker build -t ${HUB}/test-server:${TAG} -f Dockerfile .

docker.run: docker.build
	docker run ${HUB}/test-server:${TAG}

docker.push: docker.build
	docker push ${HUB}/test-server:${TAG}

##### Kube Deploy

deploy:
	kubectl apply -f <( \
	  ${ISTIO_DIR}/bin/istioctl kube-inject --hub=${ISTIO_HUB} --tag=${ISTIO_TAG} -f kubernetes/deployment.yaml | \
	  sed -e "s,${ISTIO_HUB}/proxy:,${ISTIO_HUB}/proxyv2:,g")
	kubectl apply -f <( \
	  ${ISTIO_DIR}/bin/istioctl kube-inject --hub=${ISTIO_HUB} --tag=${ISTIO_TAG} -f kubernetes/deployment-v2.yaml | \
	  sed -e "s,${ISTIO_HUB}/proxy:,${ISTIO_HUB}/proxyv2:,g")
	kubectl apply -f kubernetes/service.yaml
	kubectl apply -f kubernetes/service-v2.yaml
	kubectl apply -f kubernetes/ingress.yaml

deploy.istio:
	kubectl apply -f ${ISTIO_DIR}/install/kubernetes/istio.yaml

deploy-all: deploy.istio deploy

##### Kube Delete

remove:
	kubectl delete -f kubernetes/ || true

remove.istio:
	kubectl delete -f ${ISTIO_DIR}/install/kubernetes/istio.yaml || true

remove-all: remove remove.istio
