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

package controller

import (
	"context"

	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	ANNOTATION_INGRESS_PUBLIC = "infra.d464.sh/ingress-public"
	INGRESS_CLASS_PUBLIC      = "nginx-external"
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

	ing := &netv1.Ingress{}
	if err := r.Get(ctx, req.NamespacedName, ing); err != nil {
		if errors.IsNotFound(err) {
			l.Info("Ingress resource not found. Ignoring since object must be deleted")
		} else {
			l.Error(err, "Failed to get Ingress")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	l.Info("Ingress", "name", ing.Name, "namespace", ing.Namespace, "annotations", ing.ObjectMeta.Annotations)

	if ing.ObjectMeta.Annotations[ANNOTATION_INGRESS_PUBLIC] == "true" {
		ingc := ing.DeepCopy()

		class := INGRESS_CLASS_PUBLIC
		annotations := ingc.ObjectMeta.Annotations
		annotations[ANNOTATION_INGRESS_PUBLIC] = "false"
		spec := ingc.Spec
		spec.IngressClassName = &class

		ingp := netv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:        ing.ObjectMeta.Name + "-public",
				Namespace:   ing.ObjectMeta.Namespace,
				Annotations: annotations,
				Labels:      ing.ObjectMeta.Labels,
			},
			Spec: spec,
		}
		if err := controllerutil.SetControllerReference(ing, &ingp, r.Scheme); err != nil {
			l.Error(err, "Failed to set controller reference")
			return ctrl.Result{}, err
		}

		l.Info("Creating Ingress", "name", ingp.Name, "namespace", ingp.Namespace)
		if err := r.Create(ctx, &ingp); err != nil {
			l.Error(err, "Failed to create Ingress")
			return ctrl.Result{}, err
		}
		l.Info("Ingress created", "name", ingp.Name, "namespace", ingp.Namespace)
	} else {
		l.Info("Ingress is not public", "name", ing.Name, "namespace", ing.Namespace, "ingress-public", ing.ObjectMeta.Annotations[ANNOTATION_INGRESS_PUBLIC])
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&netv1.Ingress{}).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		// For().
		Complete(r)
}
