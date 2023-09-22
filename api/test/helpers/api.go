/*
Copyright 2023 Red Hat
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package helpers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/gophercloud/gophercloud/openstack/identity/v3/domains"
	"github.com/gophercloud/gophercloud/openstack/identity/v3/users"
	"golang.org/x/exp/maps"

	api "github.com/openstack-k8s-operators/lib-common/modules/test/apis"
)

// KeystoneAPIFixture is simulator for the OpenStack Keystone API for EnvTest
// You can simulate the happy path with the following code snippet:
//
//	f := NewKeystoneAPIFixtureWithServer(logger)
//	f.Setup()
//	DeferCleanup(f.Cleanup)
//
//	 name := th.CreateKeystoneAPIWithFixture(namespace, f)
//	DeferCleanup(th.DeleteKeystoneAPI, name)
//
// But you can also inject failures by passing custom handlers to Setup:
//
//	f := NewKeystoneAPIFixtureWithServer(logger)
//	f.Setup(
//	    Handler{Patter: "/", Func: f.HandleVersion},
//	    Handler{Patter: "/v3/auth/tokens", Func: f.HandleToken},
//	    Handler{Patter: "/v3/users", Func: func(w http.ResponseWriter, r *http.Request) {
//	        switch r.Method {
//	        case "GET":
//	            f.GetUsers(w, r)
//	        case "POST":
//	            w.WriteHeader(409)
//	        }
//	    }},
//	)
//	DeferCleanup(f.Cleanup)
type KeystoneAPIFixture struct {
	api.APIFixture
	// Users is the map of user objects known by the fixture keyed by the user
	// name
	Users map[string]users.User
	// Domains is the map of domain objects known by the fixture keyed by the
	// domain name
	Domains map[string]domains.Domain
}

// NewKeystoneAPIFixtureWithServer set up a keystone-api simulator with an
// embedded http server
func NewKeystoneAPIFixtureWithServer(log logr.Logger) *KeystoneAPIFixture {
	server := &api.FakeAPIServer{}
	server.Setup(log)
	fixture := AddKeystoneAPIFixture(log, server)
	fixture.OwnsServer = true
	return fixture
}

// AddKeystoneAPIFixture adds a keystone-api simulator to an already created
// http server. This is useful if you need to have more than one openstack API
// simulated but you don't want to start a separate http server for each.
func AddKeystoneAPIFixture(log logr.Logger, server *api.FakeAPIServer) *KeystoneAPIFixture {
	fixture := &KeystoneAPIFixture{
		APIFixture: api.APIFixture{
			Server:     server,
			Log:        log,
			URLBase:    "/identity",
			OwnsServer: false,
		},
		Users:   map[string]users.User{},
		Domains: map[string]domains.Domain{},
	}
	return fixture
}

// Setup adds the API request handlers to the fixture. If no handlers is passed
// then a basic set of well behaving handlers are added that will simulate the
// happy path.
// If you need to customize the behavior of the fixture, e.g. to inject faults,
// then you can pass a list of handlers to register instead.
func (f *KeystoneAPIFixture) Setup(handlers ...api.Handler) {
	if len(handlers) == 0 {
		f.registerNormalHandlers()
	}
	for _, handler := range handlers {
		f.registerHandler(handler)
	}
}

func (f *KeystoneAPIFixture) registerNormalHandlers() {
	f.registerHandler(api.Handler{Pattern: "/", Func: f.HandleVersion})
	f.registerHandler(api.Handler{Pattern: "/v3/auth/tokens", Func: f.HandleToken})
	f.registerHandler(api.Handler{Pattern: "/v3/users", Func: f.HandleUsers})
	f.registerHandler(api.Handler{Pattern: "/v3/domains", Func: f.HandleDomains})
}

func (f *KeystoneAPIFixture) registerHandler(handler api.Handler) {
	f.Server.AddHandler(f.URLBase+handler.Pattern, handler.Func)
}

// HandleVersion responds with a valid keystone version response
func (f *KeystoneAPIFixture) HandleVersion(w http.ResponseWriter, r *http.Request) {
	f.LogRequest(r)
	// The /identity URL matches to every request if no handle registered with a more
	// specific URL pattern
	if r.RequestURI != f.URLBase+"/" {
		f.UnexpectedRequest(w, r)
		return
	}

	switch r.Method {
	case "GET":
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprintf(w,
			`
			{
				"versions":{
				   "values":[
					  {
						 "id":"v3.14",
						 "status":"stable",
						 "links":[
							{
							   "rel":"self",
							   "href":"%s/v3/"
							}
						 ],
						 "media-types":[]
					  }
				   ]
				}
			 }
			 `, f.Endpoint())
	default:
		f.UnexpectedRequest(w, r)
		return
	}
}

// HandleToken responds with a valid keystone token
func (f *KeystoneAPIFixture) HandleToken(w http.ResponseWriter, r *http.Request) {
	f.LogRequest(r)
	switch r.Method {
	case "POST":
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(202)
		fmt.Fprintf(w,
			`
			{
				"token":{
				   "catalog":[
					  {
						 "endpoints":[
							{
							   "id":"e6ec29ecce164c3084ef308478080127",
							   "interface":"public",
							   "region_id":"RegionOne",
							   "url":"%s",
							   "region":"RegionOne"
							}
						 ],
						 "id":"edad7277e52a47b3bfb2b7004f77110f",
						 "type":"identity",
						 "name":"keystone"
					  }
				   ]
				}
			 }
			`, f.Endpoint())
	default:
		f.UnexpectedRequest(w, r)
		return
	}
}

// HandleUsers handles the happy path of GET /v3/users and POST /v3/users API
func (f *KeystoneAPIFixture) HandleUsers(w http.ResponseWriter, r *http.Request) {
	f.LogRequest(r)
	switch r.Method {
	case "GET":
		f.GetUsers(w, r)
	case "POST":
		f.CreateUser(w, r)
	default:
		f.UnexpectedRequest(w, r)
		return
	}
}

// GetUsers handles GET /v3/users based on the fixture internal state
func (f *KeystoneAPIFixture) GetUsers(w http.ResponseWriter, r *http.Request) {
	nameFilter := r.URL.Query().Get("name")
	var us []users.User
	if nameFilter == "" {
		us = maps.Values(f.Users)
	} else {
		for name, user := range f.Users {
			if name == nameFilter {
				us = append(us, user)
			}
		}
	}

	var s struct {
		Users []users.User `json:"users"`
	}
	s.Users = us

	bytes, err := json.Marshal(&s)
	if err != nil {
		f.InternalError(err, "Error during marshalling response", w, r)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(200)
	fmt.Fprint(w, string(bytes))
}

// CreateUser handles POST /v3/users and records the created user in memory
func (f *KeystoneAPIFixture) CreateUser(w http.ResponseWriter, r *http.Request) {
	bytes, err := io.ReadAll(r.Body)
	if err != nil {
		f.InternalError(err, "Error reading request body", w, r)
		return
	}
	var s struct {
		User users.User `json:"user"`
	}
	err = json.Unmarshal(bytes, &s)
	if err != nil {
		f.InternalError(err, "Error during unmarshalling request", w, r)
		return
	}
	if s.User.ID == "" {
		s.User.ID = uuid.NewString()
	}

	f.Users[s.User.Name] = s.User

	bytes, err = json.Marshal(&s)
	if err != nil {
		f.InternalError(err, "Error during marshalling response", w, r)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(201)
	fmt.Fprint(w, string(bytes))
}

// HandleDomains handles the happy path of GET /v3/domains and POST /v3/domains API
func (f *KeystoneAPIFixture) HandleDomains(w http.ResponseWriter, r *http.Request) {
	f.LogRequest(r)
	switch r.Method {
	case "GET":
		f.GetDomains(w, r)
	case "POST":
		f.CreateDomain(w, r)
	default:
		f.UnexpectedRequest(w, r)
		return
	}
}

// GetDomains handles GET /v3/domains based on the fixture internal state
func (f *KeystoneAPIFixture) GetDomains(w http.ResponseWriter, r *http.Request) {
	nameFilter := r.URL.Query().Get("name")
	var ds []domains.Domain
	if nameFilter == "" {
		ds = maps.Values(f.Domains)
	} else {
		for name, domain := range f.Domains {
			if name == nameFilter {
				ds = append(ds, domain)
			}
		}
	}

	var s struct {
		Domains []domains.Domain `json:"domains"`
	}
	s.Domains = ds

	bytes, err := json.Marshal(&s)
	if err != nil {
		f.InternalError(err, "Error during marshalling response", w, r)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(200)
	fmt.Fprint(w, string(bytes))
}

// CreateDomain handles POST /v3/domains and records the created domain in memory
func (f *KeystoneAPIFixture) CreateDomain(w http.ResponseWriter, r *http.Request) {
	bytes, err := io.ReadAll(r.Body)
	if err != nil {
		f.InternalError(err, "Error reading request body", w, r)
		return
	}
	var s struct {
		Domain domains.Domain `json:"domain"`
	}
	err = json.Unmarshal(bytes, &s)
	if err != nil {
		f.InternalError(err, "Error during unmarshalling request", w, r)
		return
	}
	if s.Domain.ID == "" {
		s.Domain.ID = uuid.NewString()
	}

	f.Domains[s.Domain.Name] = s.Domain

	bytes, err = json.Marshal(&s)
	if err != nil {
		f.InternalError(err, "Error during marshalling response", w, r)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(201)
	fmt.Fprint(w, string(bytes))
}
