/*
Copyright 2022 Red Hat

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

// +kubebuilder:object:generate:=true

package v1beta1

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestFilterServices(t *testing.T) {

	tests := []struct {
		name              string
		keystoneEndpoints []KeystoneEndpoint
		servicesToKeep    []string
		want              []KeystoneEndpoint
	}{
		{
			name:              "No data",
			keystoneEndpoints: []KeystoneEndpoint{},
			servicesToKeep:    []string{},
			want:              []KeystoneEndpoint{},
		},
		{
			name: "No common.AppSelector label on endpoint",
			keystoneEndpoints: []KeystoneEndpoint{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "endpt1",
						Namespace: "namespace",
					},
				},
			},
			servicesToKeep: []string{},
			want:           []KeystoneEndpoint{},
		},
		{
			name: "No filter",
			keystoneEndpoints: []KeystoneEndpoint{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "endpt1",
						Namespace: "namespace",
						Labels: map[string]string{
							common.AppSelector: "endpt1",
						},
					},
				},
			},
			servicesToKeep: []string{},
			want:           []KeystoneEndpoint{},
		},
		{
			name: "with filter",
			keystoneEndpoints: []KeystoneEndpoint{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "endpt1",
						Namespace: "namespace",
						Labels: map[string]string{
							common.AppSelector: "endpt1",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "endpt2",
						Namespace: "namespace",
						Labels: map[string]string{
							common.AppSelector: "endpt2",
						},
					},
				},
			},
			servicesToKeep: []string{"endpt1"},
			want: []KeystoneEndpoint{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "endpt1",
						Namespace: "namespace",
						Labels: map[string]string{
							common.AppSelector: "endpt1",
						},
					},
				},
			},
		},
		{
			name: "with multiple entries in filter",
			keystoneEndpoints: []KeystoneEndpoint{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "endpt1",
						Namespace: "namespace",
						Labels: map[string]string{
							common.AppSelector: "endpt1",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "endpt2",
						Namespace: "namespace",
						Labels: map[string]string{
							common.AppSelector: "endpt2",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "endpt3",
						Namespace: "namespace",
						Labels: map[string]string{
							common.AppSelector: "endpt3",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "endpt4",
						Namespace: "namespace",
						Labels: map[string]string{
							common.AppSelector: "endpt4",
						},
					},
				},
			},
			servicesToKeep: []string{"endpt1", "endpt3"},
			want: []KeystoneEndpoint{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "endpt1",
						Namespace: "namespace",
						Labels: map[string]string{
							common.AppSelector: "endpt1",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "endpt3",
						Namespace: "namespace",
						Labels: map[string]string{
							common.AppSelector: "endpt3",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			endpts := filterServices(tt.keystoneEndpoints, tt.servicesToKeep)
			g.Expect(endpts).To(Equal(tt.want))
		})
	}
}

func TestFilterVisibility(t *testing.T) {

	tests := []struct {
		name              string
		keystoneEndpoints []KeystoneEndpoint
		visibility        *string
		want              []string
	}{
		{
			name:              "No data",
			keystoneEndpoints: []KeystoneEndpoint{},
			visibility:        nil,
			want:              []string{},
		},
		{
			name: "with filter",
			keystoneEndpoints: []KeystoneEndpoint{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "endpt1",
						Namespace: "namespace",
						Labels: map[string]string{
							common.AppSelector: "endpt1",
						},
					},
					Status: KeystoneEndpointStatus{
						Endpoints: []Endpoint{
							{
								Interface: "internal",
								URL:       "endpt1-internal",
							},
							{
								Interface: "public",
								URL:       "endpt1-public",
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "endpt2",
						Namespace: "namespace",
						Labels: map[string]string{
							common.AppSelector: "endpt2",
						},
					},
					Status: KeystoneEndpointStatus{
						Endpoints: []Endpoint{
							{
								Interface: "internal",
								URL:       "endpt2-internal",
							},
							{
								Interface: "public",
								URL:       "endpt2-public",
							},
						},
					},
				},
			},
			visibility: nil,
			want: []string{
				"endpt1-internal",
				"endpt1-public",
				"endpt2-internal",
				"endpt2-public",
			},
		},
		{
			name: "with filter",
			keystoneEndpoints: []KeystoneEndpoint{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "endpt1",
						Namespace: "namespace",
						Labels: map[string]string{
							common.AppSelector: "endpt1",
						},
					},
					Status: KeystoneEndpointStatus{
						Endpoints: []Endpoint{
							{
								Interface: "internal",
								URL:       "endpt1-internal",
							},
							{
								Interface: "public",
								URL:       "endpt1-public",
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "endpt2",
						Namespace: "namespace",
						Labels: map[string]string{
							common.AppSelector: "endpt2",
						},
					},
					Status: KeystoneEndpointStatus{
						Endpoints: []Endpoint{
							{
								Interface: "internal",
								URL:       "endpt2-internal",
							},
							{
								Interface: "public",
								URL:       "endpt2-public",
							},
						},
					},
				},
			},
			visibility: ptr.To("internal"),
			want: []string{
				"endpt1-internal",
				"endpt2-internal",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			endpts := filterVisibility(tt.keystoneEndpoints, tt.visibility)
			g.Expect(endpts).To(Equal(tt.want))
		})
	}
}
