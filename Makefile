NAME = exporter-discovery
IMAGE = ghcr.io/xonvanetta/${NAME}

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

helm-package:
	@if [ -z "$(TAG)" ]; then echo "TAG is required. Usage: make helm-push TAG=v1.0.0"; exit 1; fi
	sed -i "s/^version:.*/version: $(TAG:v%=%)/" helm/Chart.yaml
	sed -i "s/^appVersion:.*/appVersion: \"$(TAG:v%=%)\"/" helm/Chart.yaml
	helm package helm/

helm-push: helm-package
	helm push ${NAME}-$(TAG:v%=%).tgz oci://ghcr.io/xonvanetta
