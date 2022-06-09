/*
Copyright 2021 The cert-manager Authors.

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

package webhook

import (
	"context"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2/klogr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	trustapi "github.com/cert-manager/trust/pkg/apis/trust/v1alpha1"
)

func Test_Handle(t *testing.T) {
	tests := map[string]struct {
		req     admission.Request
		expResp admission.Response
	}{
		"a request with no kind sent should return an Error response": {
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					UID:       types.UID("abc"),
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: []byte(`
{
 "apiVersion": "trust.cert-manager.io/v1alpha1",
	"kind": "NotBundle",
	"metadata": {
		"name": "testing"
	},
}
`),
					},
				},
			},

			expResp: admission.Response{
				AdmissionResponse: admissionv1.AdmissionResponse{
					Allowed: false,
					Result:  &metav1.Status{Message: "no resource kind sent in request", Code: 400},
				},
			},
		},
		"a resource who's type is not recognised should return a Denied response": {
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					UID: types.UID("abc"),
					RequestKind: &metav1.GroupVersionKind{
						Group:   "trust.cert-manager.io",
						Version: "v1alpha1",
						Kind:    "NotBundle",
					},
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: []byte(`
{
	"apiVersion": "trust.cert-manager.io/v1alpha1",
	 "kind": "NotBundle",
	 "metadata": {
	 	"name": "testing"
	 },
}
		`),
					},
				},
			},

			expResp: admission.Response{
				AdmissionResponse: admissionv1.AdmissionResponse{
					Allowed: false,
					Result:  &metav1.Status{Reason: "validation request for unrecognised resource type: trust.cert-manager.io/v1alpha1 NotBundle", Code: 403},
				},
			},
		},
		"a Bundle that fails to decode should return an Error response": {
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					UID: types.UID("abc"),
					RequestKind: &metav1.GroupVersionKind{
						Group:   "trust.cert-manager.io",
						Version: "v1alpha1",
						Kind:    "Bundle",
					},
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: []byte(`
{
 "apiVersion": "trust.cert-manager.io/v1alpha1",
	"kind": "Bundle",
	"metadata": {
		"name": "testing"
	},
	"spec": {
	  "foo": "bar",
	}
}
`),
					},
				},
			},
			expResp: admission.Response{
				AdmissionResponse: admissionv1.AdmissionResponse{
					Allowed: false,
					Result:  &metav1.Status{Message: "couldn't get version/kind; json parse error: invalid character '}' looking for beginning of object key string", Code: 400},
				},
			},
		},
		"a Bundle which fails validation should return a Denied response": {
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					UID: types.UID("abc"),
					RequestKind: &metav1.GroupVersionKind{
						Group:   "trust.cert-manager.io",
						Version: "v1alpha1",
						Kind:    "Bundle",
					},
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: []byte(`
{
 "apiVersion": "trust.cert-manager.io/v1alpha1",
	"kind": "Bundle",
	"metadata": {
		"name": "testing"
	},
	"spec": {
		"sources": [],
		"target": {
		  "configMap": {
			  "key": "bar"
			}
		}
	}
}
`),
					},
				},
			},
			expResp: admission.Response{
				AdmissionResponse: admissionv1.AdmissionResponse{
					Allowed: false,
					Result:  &metav1.Status{Reason: "spec.sources: Forbidden: must define at least one source", Code: 403},
				},
			},
		},
		"a Bundle which succeeds validation should return an Allowed response": {
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					UID: types.UID("abc"),
					RequestKind: &metav1.GroupVersionKind{
						Group:   "trust.cert-manager.io",
						Version: "v1alpha1",
						Kind:    "Bundle",
					},
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: []byte(`
{
 "apiVersion": "trust.cert-manager.io/v1alpha1",
	"kind": "Bundle",
	"metadata": {
		"name": "testing"
	},
	"spec": {
		"sources": [{ "inLine": "foo" }],
		"target": {
		  "configMap": {
			  "key": "bar"
			},
			"namespaceSelector": {
			  "foo": "bar"
			}
		}
	}
}
`),
					},
				},
			},
			expResp: admission.Response{
				AdmissionResponse: admissionv1.AdmissionResponse{
					Allowed: true,
					Result:  &metav1.Status{Reason: "Bundle validated", Code: 200},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			decoder, err := admission.NewDecoder(trustapi.GlobalScheme)
			if err != nil {
				t.Fatal(err)
			}

			v := &validator{decoder: decoder, log: klogr.New()}
			resp := v.Handle(context.TODO(), test.req)
			if !apiequality.Semantic.DeepEqual(test.expResp, resp) {
				t.Errorf("unexpected validate admission response: exp=%+v got=%+v", test.expResp, resp)
			}
		})
	}
}

func Test_validateBundle(t *testing.T) {
	var (
		nilKeySelector *trustapi.KeySelector
	)

	tests := map[string]struct {
		bundle *trustapi.Bundle
		expEl  field.ErrorList
	}{
		"no sources, no target": {
			bundle: &trustapi.Bundle{
				Spec: trustapi.BundleSpec{},
			},
			expEl: field.ErrorList{
				field.Forbidden(field.NewPath("spec", "sources"), "must define at least one source"),
				field.Invalid(field.NewPath("spec", "target", "configMap"), nilKeySelector, "target configMap must be defined"),
			},
		},
		"sources with multiple types defined in items": {
			bundle: &trustapi.Bundle{
				Spec: trustapi.BundleSpec{
					Sources: []trustapi.BundleSource{
						{
							ConfigMap: &trustapi.SourceObjectKeySelector{Name: "test", KeySelector: trustapi.KeySelector{Key: "test"}},
							InLine:    pointer.String("test"),
						},
						{InLine: pointer.String("test")},
						{
							ConfigMap: &trustapi.SourceObjectKeySelector{Name: "test", KeySelector: trustapi.KeySelector{Key: "test"}},
							Secret:    &trustapi.SourceObjectKeySelector{Name: "test", KeySelector: trustapi.KeySelector{Key: "test"}},
						},
					},
					Target: trustapi.BundleTarget{ConfigMap: &trustapi.KeySelector{Key: "test"}},
				},
			},
			expEl: field.ErrorList{
				field.Forbidden(field.NewPath("spec", "sources", "[0]"), "must define exactly one source type for each item"),
				field.Forbidden(field.NewPath("spec", "sources", "[2]"), "must define exactly one source type for each item"),
			},
		},
		"sources no names and keys": {
			bundle: &trustapi.Bundle{
				Spec: trustapi.BundleSpec{
					Sources: []trustapi.BundleSource{
						{ConfigMap: &trustapi.SourceObjectKeySelector{Name: "", KeySelector: trustapi.KeySelector{Key: ""}}},
						{InLine: pointer.String("test")},
						{Secret: &trustapi.SourceObjectKeySelector{Name: "", KeySelector: trustapi.KeySelector{Key: ""}}},
					},
					Target: trustapi.BundleTarget{ConfigMap: &trustapi.KeySelector{Key: "test"}},
				},
			},
			expEl: field.ErrorList{
				field.Invalid(field.NewPath("spec", "sources", "[0]", "configMap", "name"), "", "source configMap name must be defined"),
				field.Invalid(field.NewPath("spec", "sources", "[0]", "configMap", "key"), "", "source configMap key must be defined"),
				field.Invalid(field.NewPath("spec", "sources", "[2]", "secret", "name"), "", "source secret name must be defined"),
				field.Invalid(field.NewPath("spec", "sources", "[2]", "secret", "key"), "", "source secret key must be defined"),
			},
		},
		"sources defines the same configMap target": {
			bundle: &trustapi.Bundle{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bundle"},
				Spec: trustapi.BundleSpec{
					Sources: []trustapi.BundleSource{
						{InLine: pointer.String("test")},
						{ConfigMap: &trustapi.SourceObjectKeySelector{Name: "test-bundle", KeySelector: trustapi.KeySelector{Key: "test"}}},
					},
					Target: trustapi.BundleTarget{ConfigMap: &trustapi.KeySelector{Key: "test"}},
				},
			},
			expEl: field.ErrorList{
				field.Forbidden(field.NewPath("spec", "sources", "[1]", "configMap", "test-bundle", "test"), "cannot define the same source as target"),
			},
		},
		"target configMap key not defined": {
			bundle: &trustapi.Bundle{
				Spec: trustapi.BundleSpec{
					Sources: []trustapi.BundleSource{
						{InLine: pointer.String("test")},
					},
					Target: trustapi.BundleTarget{ConfigMap: &trustapi.KeySelector{Key: ""}},
				},
			},
			expEl: field.ErrorList{
				field.Invalid(field.NewPath("spec", "target", "configMap", "key"), "", "target configMap key must be defined"),
			},
		},
		"conditions with the same type": {
			bundle: &trustapi.Bundle{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bundle-1"},
				Spec: trustapi.BundleSpec{
					Sources: []trustapi.BundleSource{
						{InLine: pointer.String("test-1")},
					},
					Target: trustapi.BundleTarget{ConfigMap: &trustapi.KeySelector{Key: "test-1"}},
				},
				Status: trustapi.BundleStatus{
					Conditions: []trustapi.BundleCondition{
						{
							Type:   "A",
							Reason: "B",
						},
						{
							Type:   "A",
							Reason: "C",
						},
					},
				},
			},
			expEl: field.ErrorList{
				field.Invalid(field.NewPath("status", "conditions", "[1]"), trustapi.BundleCondition{Type: "A", Reason: "C"}, "condition type already present on Bundle"),
			},
		},
		"invalid namespace selector": {
			bundle: &trustapi.Bundle{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bundle-1"},
				Spec: trustapi.BundleSpec{
					Sources: []trustapi.BundleSource{
						{InLine: pointer.String("test-1")},
					},
					Target: trustapi.BundleTarget{
						ConfigMap: &trustapi.KeySelector{Key: "test-1"},
						NamespaceSelector: &trustapi.NamespaceSelector{
							MatchLabels: map[string]string{"@@@@": ""},
						},
					},
				},
				Status: trustapi.BundleStatus{
					Conditions: []trustapi.BundleCondition{
						{
							Type:   "A",
							Reason: "C",
						},
					},
				},
			},
			expEl: field.ErrorList{
				field.Invalid(field.NewPath("spec", "target", "namespaceSelector", "matchLabels"), map[string]string{"@@@@": ""}, `key: Invalid value: "@@@@": name part must consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character (e.g. 'MyName',  or 'my.name',  or '123-abc', regex used for validation is '([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]')`),
			},
		},
		"valid bundle": {
			bundle: &trustapi.Bundle{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bundle-1"},
				Spec: trustapi.BundleSpec{
					Sources: []trustapi.BundleSource{
						{InLine: pointer.String("test-1")},
					},
					Target: trustapi.BundleTarget{
						ConfigMap: &trustapi.KeySelector{Key: "test-1"},
						NamespaceSelector: &trustapi.NamespaceSelector{
							MatchLabels: map[string]string{"foo": "bar"},
						},
					},
				},
				Status: trustapi.BundleStatus{
					Conditions: []trustapi.BundleCondition{
						{
							Type:   "A",
							Reason: "B",
						},
						{
							Type:   "B",
							Reason: "C",
						},
					},
				},
			},
			expEl: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			el, err := new(validator).validateBundle(context.TODO(), test.bundle)
			if err != nil {
				t.Errorf("unexpected error: %s", err)
			}

			if !apiequality.Semantic.DeepEqual(test.expEl, el) {
				t.Errorf("unexpected errorList: exp=%v got=%v", test.expEl, el)
			}
		})
	}
}
