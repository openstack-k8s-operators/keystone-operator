# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

all: operator

csv-generator: build/_output/csv-generator
build/_output/csv-generator: tools/csv-generator.go
	CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' -o $@ $<

operator-image: build/_output/csv-generator build/_registry/image_label
	operator-sdk build --image-builder buildah $$(cat build/_registry/image_label)

buildah-login: build/_registry/account build/_registry/token build/_registry/url
	buildah login -u $$(cat build/_registry/account) --password-stdin \
	    $$(cat build/_registry/url) < build/_registry/token

buildah-push: build/_registry/image_label build/_registry/image_tag
	buildah push $$(cat build/_registry/image_label) \
	    docker://$$(cat build/_registry/image_label):$$(cat build/_registry/image_tag)

clean:
	GO111MODULE=on; \
	go mod tidy; \
	go mod vendor
	rm -rf build/_output
	rm -rf build/_registry

build: clean operator-image buildah-login buildah-push

.PHONY: \
    all csv-generator operator-image \
    buildah-login buildah-push \
    clean build \
    build/_registry/image_tag

# Targets to populate build/_registry with various data about the image and
# private registry
build/_registry:
	mkdir -p $@

build/_registry/account: | build/_registry
	oc get -n openstack sa/builder -o json | \
	    jq -r ".secrets[1].name" > $@

build/_registry/token: build/_registry/account | build/_registry
	oc get -n openstack secret/$$(cat build/_registry/account) -o json | \
	    jq -r '.metadata.annotations["openshift.io/token-secret.value"]' > $@

build/_registry/url: | build/_registry
	oc get route -n openshift-image-registry -o json | \
	    jq -r ".items[0].spec.host" > $@

# NOTE: We mark this target as PHONY to ensure it is always recalculated before
# use
build/_registry/image_tag: | build/_registry
	git rev-parse --short HEAD > $@

build/_registry/image_label: build/_registry/url | build/_registry
	echo $$(cat build/_registry/url)/openstack/keystone-operator > $@
