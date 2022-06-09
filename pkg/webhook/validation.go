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
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/cert-manager/trust/pkg/apis/trust"
	trustapi "github.com/cert-manager/trust/pkg/apis/trust/v1alpha1"
)

// validator validates against trust.cert-manager.io resources.
type validator struct {
	log logr.Logger

	decoder *admission.Decoder

	lock sync.RWMutex
}

// Handle is a Kubernetes validation webhook server handler. Returns an
// admission response containing whether the request is allowed or not.
func (v *validator) Handle(ctx context.Context, req admission.Request) admission.Response {
	log := v.log.WithValues("name", req.Name)
	log.V(2).Info("received validation request")

	if req.RequestKind == nil {
		return admission.Errored(http.StatusBadRequest, errors.New("no resource kind sent in request"))
	}

	var (
		el  field.ErrorList
		err error
	)

	switch *req.RequestKind {
	case metav1.GroupVersionKind{Group: trust.GroupName, Version: "v1alpha1", Kind: "Bundle"}:
		var bundle trustapi.Bundle

		v.lock.RLock()
		err := v.decoder.Decode(req, &bundle)
		v.lock.RUnlock()

		if err != nil {
			log.Error(err, "failed to decode Bundle")
			return admission.Errored(http.StatusBadRequest, err)
		}

		el, err = v.validateBundle(ctx, &bundle)

	default:
		return admission.Denied(fmt.Sprintf("validation request for unrecognised resource type: %s/%s %s", req.RequestKind.Group, req.RequestKind.Version, req.RequestKind.Kind))
	}

	if err != nil {
		log.Error(err, "internal error occurred validating request")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	if err := el.ToAggregate(); err != nil {
		v.log.V(2).Info("denied request", "reason", el.ToAggregate().Error())
		return admission.Denied(el.ToAggregate().Error())
	}

	log.V(2).Info("allowed request")
	return admission.Allowed("Bundle validated")
}

// validateBundle validates the incoming Bundle object and returns any
// resulting error.
func (v *validator) validateBundle(ctx context.Context, bundle *trustapi.Bundle) (field.ErrorList, error) {
	var el field.ErrorList
	path := field.NewPath("spec")

	if len(bundle.Spec.Sources) == 0 {
		el = append(el, field.Forbidden(path.Child("sources"), "must define at least one source"))
	} else {
		path := path.Child("sources")

		for i, source := range bundle.Spec.Sources {
			path := path.Child("[" + strconv.Itoa(i) + "]")
			unionCount := 0

			if configMap := source.ConfigMap; configMap != nil {
				path := path.Child("configMap")
				unionCount++

				if len(configMap.Name) == 0 {
					el = append(el, field.Invalid(path.Child("name"), configMap.Name, "source configMap name must be defined"))
				}
				if len(configMap.Key) == 0 {
					el = append(el, field.Invalid(path.Child("key"), configMap.Key, "source configMap key must be defined"))
				}
			}

			if secret := source.Secret; secret != nil {
				path := path.Child("secret")
				unionCount++

				if len(secret.Name) == 0 {
					el = append(el, field.Invalid(path.Child("name"), secret.Name, "source secret name must be defined"))
				}
				if len(secret.Key) == 0 {
					el = append(el, field.Invalid(path.Child("key"), secret.Key, "source secret key must be defined"))
				}
			}

			if source.InLine != nil {
				unionCount++
			}

			if unionCount != 1 {
				el = append(el, field.Forbidden(path, "must define exactly one source type for each item"))
			}
		}
	}

	if target := bundle.Spec.Target.ConfigMap; target != nil {
		path := path.Child("sources")
		for i, source := range bundle.Spec.Sources {
			if source.ConfigMap != nil && source.ConfigMap.Name == bundle.Name && source.ConfigMap.Key == target.Key {
				el = append(el, field.Forbidden(path.Child(fmt.Sprintf("[%d]", i), "configMap", source.ConfigMap.Name, source.ConfigMap.Key), "cannot define the same source as target"))
			}
		}
	}

	if configMap := bundle.Spec.Target.ConfigMap; configMap == nil {
		el = append(el, field.Invalid(path.Child("target", "configMap"), configMap, "target configMap must be defined"))
	} else if len(configMap.Key) == 0 {
		el = append(el, field.Invalid(path.Child("target", "configMap", "key"), configMap.Key, "target configMap key must be defined"))
	}

	if nsSel := bundle.Spec.Target.NamespaceSelector; nsSel != nil && len(nsSel.MatchLabels) > 0 {
		if _, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchLabels: nsSel.MatchLabels}); err != nil {
			el = append(el, field.Invalid(path.Child("target", "namespaceSelector", "matchLabels"), nsSel.MatchLabels, err.Error()))
		}
	}

	path = field.NewPath("status")

	conditionTypes := make(map[trustapi.BundleConditionType]struct{})
	for i, condition := range bundle.Status.Conditions {
		if _, ok := conditionTypes[condition.Type]; ok {
			el = append(el, field.Invalid(path.Child("conditions", "["+strconv.Itoa(i)+"]"), condition, "condition type already present on Bundle"))
		}
		conditionTypes[condition.Type] = struct{}{}
	}

	return el, nil
}

// InjectDecoder is used by the controller-runtime manager to inject an object
// decoder to convert into know trust.cert-manager.io types.
func (v *validator) InjectDecoder(d *admission.Decoder) error {
	v.lock.Lock()
	defer v.lock.Unlock()

	v.decoder = d
	return nil
}

// check is used by the shared readiness manager to expose whether the server
// is ready.
func (v *validator) check(_ *http.Request) error {
	v.lock.RLock()
	defer v.lock.RUnlock()

	if v.decoder != nil {
		return nil
	}

	return errors.New("not ready")
}
