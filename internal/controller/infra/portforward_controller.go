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

package infra

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrav1 "git.d464.sh/infra/operator/api/infra/v1"
	infraweb "git.d464.sh/infra/operator/internal/web"
)

// PortForwardReconciler reconciles a PortForward object
type PortForwardReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Forwards *infraweb.Forwards
}

//+kubebuilder:rbac:groups=infra.d464.sh,resources=portforwards,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infra.d464.sh,resources=portforwards/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infra.d464.sh,resources=portforwards/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the PortForward object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *PortForwardReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	key := req.Namespace + "/" + req.Name
	forward := &infrav1.PortForward{}
	err := r.Get(ctx, req.NamespacedName, forward)
	if err != nil && !errors.IsNotFound(err) {
		l.Error(err, "unable to fetch PortForward")
		return ctrl.Result{}, err
	} else if errors.IsNotFound(err) {
		r.Forwards.Delete(key)
		return ctrl.Result{}, nil
	}

	var srcPort int32
	if forward.Spec.ExternalPort == nil {
		srcPort = forward.Spec.Port
	} else {
		srcPort = *forward.Spec.ExternalPort
	}

	r.Forwards.Set(key, string(forward.Spec.Protocol), srcPort, forward.Spec.Address, forward.Spec.Port)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PortForwardReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.PortForward{}).
		Complete(r)
}
