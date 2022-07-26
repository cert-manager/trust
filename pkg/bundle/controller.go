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

package bundle

import (
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/clock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	trustapi "github.com/cert-manager/trust/pkg/apis/trust/v1alpha1"
	"github.com/cert-manager/trust/pkg/bundle/internal"
)

// AddBundleController will register the Bundle controller with the
// controller-runtime Manager.
// The Bundle controller will reconcile Bundles on Bundle events, as well as
// when any related resource event in the Bundle source and target.
// The controller will only cache metadata for ConfigMaps and Secrets.
func AddBundleController(ctx context.Context, mgr manager.Manager, opts Options) error {
	b := &bundle{
		client:    mgr.GetClient(),
		lister:    mgr.GetCache(),
		recorder:  mgr.GetEventRecorderFor("bundles"),
		clock:     clock.RealClock{},
		etagCache: newEtagCache(),
		Options:   opts,
	}

	// Only reconcile config maps that match the well known name
	if err := ctrl.NewControllerManagedBy(mgr).
		// Reconcile trust.cert-manager.io Bundles
		For(new(trustapi.Bundle)).

		// Reconcile over owned ConfigMaps in all Namespaces. Only cache metadata.
		// These ConfigMaps will be Bundle Targets
		Owns(new(corev1.ConfigMap), builder.OnlyMetadata).

		// Watch all Namespaces. Cache whole Namespaces to include Phase Status.
		// Reconcile all Bundles on a Namespace change.
		Watches(&source.Kind{Type: new(corev1.Namespace)}, handler.EnqueueRequestsFromMapFunc(
			func(obj client.Object) []reconcile.Request {
				err := b.lister.Get(ctx, client.ObjectKeyFromObject(obj), obj)
				if apierrors.IsNotFound(err) {
					// No need to reconcile all Bundles if the namespace has been deleted.
					return nil
				}

				if err != nil {
					// If an error occurred we still need to reconcile all Bundles to
					// ensure no Namespace gets lost.
					b.Log.Error(err, "failed to get Namespace, reconciling all Bundles anyway", "namespace", obj.GetName())
				}

				// If an error happens here and we do nothing, we run the risk of
				// leaving a Namespace behind when syncing.
				// Exiting error is the safest option, as it will force a resync on
				// all Bundles on start.
				bundleList := b.mustBundleList(ctx)

				var requests []reconcile.Request
				for _, bundle := range bundleList.Items {
					requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: bundle.Name}})
				}

				return requests
			},
		)).

		// Watch ConfigMaps in trust Namespace. Only cache metadata.
		// Reconcile Bundles who reference a modified source ConfigMap.
		Watches(&source.Kind{Type: new(corev1.ConfigMap)}, handler.EnqueueRequestsFromMapFunc(
			func(obj client.Object) []reconcile.Request {

				// If an error happens here and we do nothing, we run the risk of
				// having trust Bundles out of sync with this source or target
				// ConfigMap.
				// Exiting error is the safest option, as it will force a resync on
				// all Bundles on start.
				bundleList := b.mustBundleList(ctx)

				var requests []reconcile.Request
				for _, bundle := range bundleList.Items {
					for _, source := range bundle.Spec.Sources {
						if source.ConfigMap == nil {
							continue
						}

						// Bundle references this ConfigMap as a source. Add to request.
						if source.ConfigMap.Name == obj.GetName() {
							requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: bundle.Name}})
							break
						}
					}
				}

				return requests
			},
		), builder.OnlyMetadata, builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
			// Only process ConfigMaps in the trust Namespace.
			return obj.GetNamespace() == b.Namespace
		}))).

		// Watch Secrets in trust Namespace. Only cache metadata.
		// Reconcile Bundles who reference a modified source Secret.
		Watches(&source.Kind{Type: new(corev1.Secret)}, handler.EnqueueRequestsFromMapFunc(
			func(obj client.Object) []reconcile.Request {
				// If an error happens here and we do nothing, we run the risk of
				// having trust Bundles out of sync with this source Secret.
				// Exiting error is the safest option, as it will force a resync on
				// all Bundles on start.
				bundleList := b.mustBundleList(ctx)

				var requests []reconcile.Request
				for _, bundle := range bundleList.Items {
					for _, source := range bundle.Spec.Sources {
						if source.Secret == nil {
							continue
						}

						// Bundle references this Secret as a source. Add to request.
						if source.Secret.Name == obj.GetName() {
							requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: bundle.Name}})
							break
						}
					}
				}

				return requests
			},
		), builder.OnlyMetadata, builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
			// Only process Secrets in the trust Namespace.
			return obj.GetNamespace() == b.Namespace
		}))).

		// Complete controller.
		Complete(b); err != nil {
		return fmt.Errorf("failed to create Bundle controller: %s", err)
	}

	return nil
}

// mustBundleList will return a BundleList of all Bundles in the cluster. If an
// error occurs, will exit error the program.
func (b *bundle) mustBundleList(ctx context.Context) *trustapi.BundleList {
	var bundleList trustapi.BundleList
	if err := b.lister.List(ctx, &bundleList); err != nil {
		b.Log.Error(err, "failed to list all Bundles, exiting error")
		os.Exit(-1)
	}

	return &bundleList
}

// NewCacheFunc will return a multi-scoped controller-runtime NewCacheFunc
// where Secret resources will only be watched within the trust Namespace.
func NewCacheFunc(opts Options) cache.NewCacheFunc {
	return internal.NewMultiScopedCache(opts.Namespace, []schema.GroupKind{{Kind: "Secret"}})
}

// ClientDisableCacheFor returns resources which should only be watched within
// the Trust Namespace, and not at the cluster level.
func ClientDisableCacheFor() []client.Object {
	return []client.Object{new(corev1.Secret)}
}
