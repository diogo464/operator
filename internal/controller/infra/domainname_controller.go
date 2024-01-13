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

// DomainNameReconciler reconciles a DomainName object
type DomainNameReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Hosts  *infraweb.Hosts
}

//+kubebuilder:rbac:groups=infra.d464.sh,resources=domainnames,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infra.d464.sh,resources=domainnames/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infra.d464.sh,resources=domainnames/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DomainName object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *DomainNameReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	key := req.Namespace + "/" + req.Name
	domainName := &infrav1.DomainName{}
	err := r.Get(ctx, req.NamespacedName, domainName)
	if err != nil && !errors.IsNotFound(err) {
		l.Error(err, "unable to fetch DomainName")
		return ctrl.Result{}, err
	} else if errors.IsNotFound(err) {
		l.Info("DomainName not found", "key", key)
		r.Hosts.Delete(key)
		return ctrl.Result{}, nil
	}

	r.Hosts.Set(key, domainName.Spec.Address, domainName.Spec.Domain)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DomainNameReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.DomainName{}).
		Complete(r)
}
