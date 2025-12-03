NAME = exporter-discovery
IMAGE = ghcr.io/xonvanetta/${NAME}
CHART_REGISTRY = ghcr.io/xonvanetta/charts

test:
	go test ./... -v

build: test
	go mod vendor && docker build . -t ${IMAGE}:${TAG} -f build/Containerfile

run: build
	docker run ${IMAGE}:${TAG}

push: build
	docker push ${IMAGE}:${TAG}

tag:
	git tag -a ${TAG} && git push --follow-tags

dry:
	helm template helm/ -n monitoring

deploy:
	helm template helm/ -n monitoring --set image.tag=${TAG} | kubectl apply -f -

destroy:
	helm template helm/ -n monitoring --set image.tag=${TAG} | kubectl delete -f -

helm-package:
	helm package helm/ --version ${TAG} --app-version ${TAG}

helm-push: helm-package
	helm push ${NAME}-${TAG}.tgz oci://${CHART_REGISTRY}
