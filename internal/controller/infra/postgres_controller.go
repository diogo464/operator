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
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrav1 "git.d464.sh/infra/operator/api/infra/v1"
	"git.d464.sh/infra/operator/internal/ksink"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	POSTGRES_NAME_SUFFIX          = "-pg"
	POSTGRES_DBNAME               = "postgres"
	POSTGRES_PORT                 = 5432
	POSTGRES_USER                 = "postgres"
	POSTGRES_PASS_LEN             = 20
	POSTGRES_SECRET_DATABASE_HOST = "DATABASE_HOST"
	POSTGRES_SECRET_DATABASE_PORT = "DATABASE_PORT"
	POSTGRES_SECRET_DATABASE_USER = "DATABASE_USER"
	POSTGRES_SECRET_DATABASE_PASS = "DATABASE_PASS"
	POSTGRES_SECRET_DATABASE_NAME = "DATABASE_NAME"
	POSTGRES_SECRET_DATABASE_URL  = "DATABASE_URL"
)

// PostgresReconciler reconciles a Postgres object
type PostgresReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=infra.d464.sh,resources=postgres,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infra.d464.sh,resources=postgres/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infra.d464.sh,resources=postgres/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Postgres object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *PostgresReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	l.Info("Reconciling Postgres: " + req.Name + " in namespace: " + req.Namespace + "")

	pg := &infrav1.Postgres{}
	if err := r.Get(ctx, req.NamespacedName, pg); err != nil {
		if !errors.IsNotFound(err) {
			l.Error(err, "unable to fetch Postgres")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	secret, err := r.getOrCreateSecret(ctx, pg)
	if err != nil {
		l.Error(err, "unable to get or create secret")
		return ctrl.Result{}, err
	}

	_, err = r.getOrCreatePvc(ctx, pg)
	if err != nil {
		l.Error(err, "unable to get or create pvc")
		return ctrl.Result{}, err
	}

	_, err = r.getOrCreateDeployment(ctx, pg, secret)
	if err != nil {
		l.Error(err, "unable to get or create deployment")
		return ctrl.Result{}, err
	}

	_, err = r.getOrCreateService(ctx, pg)
	if err != nil {
		l.Error(err, "unable to get or create service")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *PostgresReconciler) getOrCreateSecret(ctx context.Context, pg *infrav1.Postgres) (*corev1.Secret, error) {
	l := log.FromContext(ctx)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pg.Name + POSTGRES_NAME_SUFFIX,
			Namespace: pg.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if secret.CreationTimestamp.IsZero() {
			host := pg.Name + POSTGRES_NAME_SUFFIX
			password := ksink.RandomString(POSTGRES_PASS_LEN)
			secret.Data = make(map[string][]byte)
			secret.Data[POSTGRES_SECRET_DATABASE_HOST] = []byte(host)
			secret.Data[POSTGRES_SECRET_DATABASE_PORT] = []byte(fmt.Sprint(POSTGRES_PORT))
			secret.Data[POSTGRES_SECRET_DATABASE_USER] = []byte(POSTGRES_USER)
			secret.Data[POSTGRES_SECRET_DATABASE_PASS] = []byte(password)
			secret.Data[POSTGRES_SECRET_DATABASE_NAME] = []byte(POSTGRES_DBNAME)
			secret.Data[POSTGRES_SECRET_DATABASE_URL] = []byte(makePostgresUrl(host, password))
		}
		if err := controllerutil.SetControllerReference(pg, secret, r.Scheme); err != nil {
			l.Error(err, "Failed to set controller reference")
			return err
		}
		return nil
	})
	if err != nil {
		l.Error(err, "Failed to create or update secret")
		return nil, err
	}

	return secret, nil
}

func (r *PostgresReconciler) getOrCreatePvc(ctx context.Context, pg *infrav1.Postgres) (*corev1.PersistentVolumeClaim, error) {
	l := log.FromContext(ctx)

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pg.Name + POSTGRES_NAME_SUFFIX,
			Namespace: pg.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, pvc, func() error {
		pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{
			corev1.ReadWriteOncePod,
		}
		pvc.Spec.Resources.Requests = corev1.ResourceList{
			corev1.ResourceStorage: pg.Spec.Storage.Size,
		}
		pvc.Spec.StorageClassName = &pg.Spec.Storage.StorageClassName
		if err := controllerutil.SetControllerReference(pg, pvc, r.Scheme); err != nil {
			l.Error(err, "Failed to set controller reference")
			return err
		}
		return nil
	})
	if err != nil {
		l.Error(err, "Failed to create or update pvc")
		return nil, err
	}

	return pvc, nil
}

func (r *PostgresReconciler) getOrCreateDeployment(ctx context.Context, pg *infrav1.Postgres, secret *corev1.Secret) (*appv1.Deployment, error) {
	l := log.FromContext(ctx)

	image, err := imageFromPostgres(pg)
	if err != nil {
		return nil, err
	}

	deployment := &appv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pg.Name + POSTGRES_NAME_SUFFIX,
			Namespace: pg.Namespace,
		},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		deployment.Spec.Replicas = ksink.I32Ptr(1)
		deployment.Spec.Strategy.Type = appv1.RecreateDeploymentStrategyType
		deployment.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"app": pg.Name + POSTGRES_NAME_SUFFIX,
			},
		}
		deployment.Spec.Template.ObjectMeta.Labels = map[string]string{
			"app": pg.Name + POSTGRES_NAME_SUFFIX,
		}
		deployment.Spec.Template.Spec.InitContainers = []corev1.Container{
			{
				Name:            "init",
				Image:           "busybox",
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command: []string{
					"sh",
					"-c",
					"chown -R 999:999 /var/lib/postgresql/data",
				},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      pg.Name + POSTGRES_NAME_SUFFIX,
						MountPath: "/var/lib/postgresql/data",
					},
				},
			},
		}
		deployment.Spec.Template.Spec.Containers = []corev1.Container{
			{
				Name:            pg.Name + POSTGRES_NAME_SUFFIX,
				Image:           image,
				ImagePullPolicy: corev1.PullIfNotPresent,
				Env: []corev1.EnvVar{
					{
						Name:  "POSTGRES_DB",
						Value: POSTGRES_DBNAME,
					},
					{
						Name:  "POSTGRES_USER",
						Value: POSTGRES_USER,
					},
					{
						Name: "POSTGRES_PASSWORD",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								Key: POSTGRES_SECRET_DATABASE_PASS,
								LocalObjectReference: corev1.LocalObjectReference{
									Name: pg.Name + POSTGRES_NAME_SUFFIX,
								},
							},
						},
					},
					{
						Name:  "PGDATA",
						Value: "/var/lib/postgresql/data/pgdata",
					},
				},
				Resources: pg.Spec.Resources,
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      pg.Name + POSTGRES_NAME_SUFFIX,
						MountPath: "/var/lib/postgresql/data",
					},
				},
				SecurityContext: &corev1.SecurityContext{
					RunAsUser:  ksink.I64Ptr(999),
					RunAsGroup: ksink.I64Ptr(999),
				},
			},
		}
		fsGroupChangePolicy := corev1.FSGroupChangeOnRootMismatch
		deployment.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
			FSGroup:             ksink.I64Ptr(999),
			FSGroupChangePolicy: &fsGroupChangePolicy,
		}
		deployment.Spec.Template.Spec.Volumes = []corev1.Volume{
			{
				Name: pg.Name + POSTGRES_NAME_SUFFIX,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: pg.Name + POSTGRES_NAME_SUFFIX,
					},
				},
			},
		}

		if err := controllerutil.SetControllerReference(pg, deployment, r.Scheme); err != nil {
			l.Error(err, "Failed to set controller reference")
			return err
		}

		return nil
	})
	if err != nil {
		l.Error(err, "Failed to create or update deployment")
		return nil, err
	}

	return deployment, nil
}

func (r *PostgresReconciler) getOrCreateService(ctx context.Context, pg *infrav1.Postgres) (*corev1.Service, error) {
	l := log.FromContext(ctx)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pg.Name + POSTGRES_NAME_SUFFIX,
			Namespace: pg.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		service.Spec.Ports = []corev1.ServicePort{
			{
				Name:       "postgres",
				Port:       POSTGRES_PORT,
				TargetPort: intstr.FromInt(POSTGRES_PORT),
			},
		}
		service.Spec.Selector = map[string]string{
			"app": pg.Name + POSTGRES_NAME_SUFFIX,
		}
		if err := controllerutil.SetControllerReference(pg, service, r.Scheme); err != nil {
			l.Error(err, "Failed to set controller reference")
			return err
		}

		return nil
	})
	if err != nil {
		l.Error(err, "Failed to create or update service")
		return nil, err
	}

	return service, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PostgresReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.Postgres{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.Service{}).
		Owns(&appv1.Deployment{}).
		Complete(r)
}

func makePostgresUrl(host, password string) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		POSTGRES_USER, password, host, POSTGRES_PORT, POSTGRES_DBNAME)
}

func imageFromPostgres(pg *infrav1.Postgres) (string, error) {
	if pg.Spec.Tag == "" && pg.Spec.Image == "" {
		return "", fmt.Errorf("image or tag must be specified")
	}
	if pg.Spec.Image != "" {
		return pg.Spec.Image, nil
	}
	return fmt.Sprintf("postgres:%s", pg.Spec.Tag), nil
}
