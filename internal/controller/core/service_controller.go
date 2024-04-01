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

package core

import (
	"context"
	"fmt"
	"strings"

	infrav1 "git.d464.sh/infra/operator/api/infra/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	ANNOTATION_SERVICE_DOMAIN = "service.infra.d464.sh/domain"
	ANNOTATION_SERVICE_EXPOSE = "service.infra.d464.sh/expose"

	// used on other resources to indicate the service that owns them
	ANNOTATION_SERVICE_OWNER = "service.infra.d464.sh/owner"
)

// ServiceReconciler reconciles a Service object
type ServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=services/finalizers,verbs=update
//+kubebuilder:rbac:groups=infra.d464.sh,resources=portforwards,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infra.d464.sh,resources=portforwards/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infra.d464.sh,resources=portforwards/finalizers,verbs=update
//+kubebuilder:rbac:groups=infra.d464.sh,resources=domainnames,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infra.d464.sh,resources=domainnames/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infra.d464.sh,resources=domainnames/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Service object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	service := &corev1.Service{}
	if err := r.Get(ctx, req.NamespacedName, service); err != nil {
		if !errors.IsNotFound(err) {
			l.Error(err, "Failed to get service")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !r.isLoadBalancer(service) {
		l.Info("Service " + service.Name + " is not a loadbalancer")
		return ctrl.Result{}, nil
	}

	if r.getIpAddress(service) == "" {
		l.Info("Service " + service.Name + " does not have an ip address")
		return ctrl.Result{}, nil
	}

	if err := r.reconcileDomainName(ctx, service); err != nil {
		l.Error(err, "Failed to reconcile domain names")
		return ctrl.Result{}, err
	}

	if err := r.reconcilePortForwards(ctx, service); err != nil {
		l.Error(err, "Failed to reconcile portforwards")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		Owns(&infrav1.PortForward{}).
		Owns(&infrav1.DomainName{}).
		Complete(r)
}

func (r *ServiceReconciler) reconcileDomainName(ctx context.Context, service *corev1.Service) error {
	l := log.FromContext(ctx)

	domain := r.getDomainName(service)
	domainName := &infrav1.DomainName{
		ObjectMeta: metav1.ObjectMeta{
			Name:      service.Name,
			Namespace: service.Namespace,
		},
	}

	if domain == "" {
		l.Info("Service " + service.Name + " does not have a domain name")
		if err := r.Delete(ctx, domainName); err != nil {
			return client.IgnoreNotFound(err)
		}
	} else {
		l.Info("Service " + service.Name + " has domain name " + domain)
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, domainName, func() error {
			domainName.Spec.Domain = domain
			domainName.Spec.Address = r.getIpAddress(service)
			if err := controllerutil.SetOwnerReference(service, domainName, r.Scheme); err != nil {
				l.Error(err, "Failed to set owner reference")
				return err
			}
			return nil
		})
		return err
	}

	return nil
}

func (r *ServiceReconciler) reconcilePortForwards(ctx context.Context, service *corev1.Service) error {
	l := log.FromContext(ctx)

	// TODO: find better way to filter
	allPortForwards := &infrav1.PortForwardList{}
	if err := r.List(ctx, allPortForwards, client.InNamespace(service.Namespace)); err != nil {
		l.Error(err, "Failed to list portforwards")
		return err
	}
	portForwards := []*infrav1.PortForward{}
	for _, portforward := range allPortForwards.Items {
		if portforward.ObjectMeta.Annotations == nil {
			continue
		}
		if owner, ok := portforward.ObjectMeta.Annotations[ANNOTATION_SERVICE_OWNER]; ok && owner == service.Name {
			portForwards = append(portForwards, &portforward)
		}
	}

	deleteQueue := []*infrav1.PortForward{}

	if r.isExposed(service) {
		l.Info("Service " + service.Name + " is exposed and has " + fmt.Sprintf("%d", len(service.Spec.Ports)) + " ports")
		pfuids := map[types.UID]struct{}{}
		for _, port := range service.Spec.Ports {
			portforwardName := fmt.Sprintf("%s-%s-%d", service.Name, strings.ToLower(string(port.Protocol)), port.Port)
			portforward := &infrav1.PortForward{
				ObjectMeta: metav1.ObjectMeta{
					Name:      portforwardName,
					Namespace: service.Namespace,
				},
			}

			l.Info("Reconciling portforward " + portforwardName)
			op, err := controllerutil.CreateOrUpdate(ctx, r.Client, portforward, func() error {
				if portforward.ObjectMeta.Annotations == nil {
					portforward.ObjectMeta.Annotations = map[string]string{}
				}
				portforward.ObjectMeta.Annotations[ANNOTATION_SERVICE_OWNER] = service.Name
				portforward.Spec.Address = r.getIpAddress(service)
				portforward.Spec.Port = port.Port
				portforward.Spec.Protocol = port.Protocol
				if err := controllerutil.SetOwnerReference(service, portforward, r.Scheme); err != nil {
					l.Error(err, "Failed to set owner reference")
					return err
				}
				return nil
			})
			l.Info("Portforward " + portforwardName + " " + string(op))
			if err != nil {
				l.Error(err, "Failed to create or update portforward")
				return err
			}
			pfuids[portforward.UID] = struct{}{}
		}
		for _, portforward := range portForwards {
			if _, ok := pfuids[portforward.UID]; !ok {
				deleteQueue = append(deleteQueue, portforward)
			}
		}
	} else {
		l.Info("Service " + service.Name + " is not exposed")
		for _, portforward := range allPortForwards.Items {
			deleteQueue = append(deleteQueue, &portforward)
		}
	}

	for _, portforward := range deleteQueue {
		l.Info("Deleting portforward " + portforward.Name)
		if err := r.Delete(ctx, portforward); err != nil {
			l.Error(err, "Failed to delete portforward")
			if !errors.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}

func (r *ServiceReconciler) isExposed(service *corev1.Service) bool {
	if expose, ok := service.Annotations[ANNOTATION_SERVICE_EXPOSE]; ok {
		return expose == "true"
	}
	return false
}

func (r *ServiceReconciler) getDomainName(service *corev1.Service) string {
	if domain, ok := service.Annotations[ANNOTATION_SERVICE_DOMAIN]; ok {
		return domain
	}
	return ""
}

func (r *ServiceReconciler) getIpAddress(service *corev1.Service) string {
	if service.Status.LoadBalancer.Ingress == nil || len(service.Status.LoadBalancer.Ingress) == 0 {
		return ""
	}
	return service.Status.LoadBalancer.Ingress[0].IP
}

func (r *ServiceReconciler) isLoadBalancer(service *corev1.Service) bool {
	return service.Spec.Type == corev1.ServiceTypeLoadBalancer
}
