/*
Copyright 2023.

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

package networking

import (
	"context"
	"strings"

	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// the ingress class to use for the public ingress
	INGRESS_CLASS_PUBLIC = "nginx-external"

	ANNOTATION_INGRESS_FORCE_SSL = "infra.d464.sh/ingress-force-ssl"
	ANNOTATION_INGRESS_PUBLIC    = "infra.d464.sh/ingress-public"

	ANNOTATION_INGRESS_NGINX_FORCE_SSL = "nginx.ingress.kubernetes.io/force-ssl-redirect"
	// annotation to place on the private ingress to exclude it from external-dns
	ANNOTATION_INGRESS_EXTERNAL_DNS_EXCLUDE = "external-dns.alpha.kubernetes.io/exclude"
)

var (
	// prefixes to filter out of the public ingress annotations
	ANNOTATION_PUBLIC_FILTER_PREFIXES = []string{
		"hajimari.io",
		ANNOTATION_INGRESS_PUBLIC, // delete this to prevent infinite recursion
	}
)

// IngressReconciler reconciles a Ingress object
type IngressReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Ingress object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	ingress := &netv1.Ingress{}

	if strings.HasSuffix(req.Name, "-public") {
		req.Name = strings.TrimSuffix(req.Name, "-public")
	}

	if err := r.Get(ctx, req.NamespacedName, ingress); err == nil {
		return r.ReconcileIngress(ctx, ingress)
	} else {
		l.Error(err, "Failed to get ingress")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&netv1.Ingress{}).
		Complete(r)
}

func (r *IngressReconciler) ReconcileIngress(ctx context.Context, ing *netv1.Ingress) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.Info("Reconcile ingress", "name", ing.Name, "namespace", ing.Namespace, "annotations", ing.ObjectMeta.Annotations)

	l.Info("Annotations for "+ing.Name, "annotations", ing.Annotations)
	ing.Annotations = r.expandAnnotations(ing.Annotations, false)
	l.Info("Expanded annotations for "+ing.Name, "annotations", ing.Annotations)
	if err := r.Client.Update(ctx, ing); err != nil {
		l.Error(err, "Failed to update Ingress")
		return ctrl.Result{}, err
	}

	if ing.ObjectMeta.Annotations[ANNOTATION_INGRESS_PUBLIC] == "true" {
		l.Info("Ingress is public", "name", ing.Name, "namespace", ing.Namespace)
		publicIngress := &netv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: ing.ObjectMeta.Name + "-public", Namespace: ing.ObjectMeta.Namespace}}
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, publicIngress, func() error {
			reference := ing.DeepCopy()
			publicIngressClass := INGRESS_CLASS_PUBLIC
			publicIngress.ObjectMeta.Labels = reference.ObjectMeta.Labels
			publicIngress.ObjectMeta.Annotations = r.expandAnnotations(reference.ObjectMeta.Annotations, true)
			publicIngress.Spec = reference.Spec
			publicIngress.Spec.IngressClassName = &publicIngressClass
			if err := controllerutil.SetControllerReference(ing, publicIngress, r.Scheme); err != nil {
				l.Error(err, "Failed to set controller reference")
				return err
			}

			return nil
		})

		if err != nil {
			l.Error(err, "Failed to create or update Ingress")
			return ctrl.Result{}, err
		}
	} else {
		l.Info("Ingress is not public", "name", ing.Name, "namespace", ing.Namespace)
		if err := r.Delete(ctx, &netv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: ing.ObjectMeta.Name + "-public", Namespace: ing.ObjectMeta.Namespace}}); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}

	return ctrl.Result{}, nil
}

func (r *IngressReconciler) expandAnnotations(annotations map[string]string, isPublic bool) map[string]string {
	expanded := make(map[string]string)
	for key, value := range annotations {
		expanded[key] = value
	}

	if isPublic {
		for key := range annotations {
			for _, prefix := range ANNOTATION_PUBLIC_FILTER_PREFIXES {
				if strings.HasPrefix(key, prefix) {
					delete(expanded, key)
				}
			}
		}
		expanded[ANNOTATION_INGRESS_EXTERNAL_DNS_EXCLUDE] = "false"
	} else {
		// we don't want to expose the private ingress to external-dns
		expanded[ANNOTATION_INGRESS_EXTERNAL_DNS_EXCLUDE] = "true"
	}

	if v, ok := expanded[ANNOTATION_INGRESS_FORCE_SSL]; ok {
		if v == "true" {
			expanded[ANNOTATION_INGRESS_NGINX_FORCE_SSL] = "true"
		} else {
			expanded[ANNOTATION_INGRESS_NGINX_FORCE_SSL] = "false"
		}
	}

	return expanded
}
