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
	"encoding/base64"
	"encoding/json"
	"math/rand"
	"slices"
	"strings"

	"github.com/minio/madmin-go"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrav1 "git.d464.sh/infra/operator/api/infra/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	MINIO_ACCESS_KEY_PREFIX     = "infra-"
	MINIO_ACCESS_KEY_MAX_LENGTH = 20
)

type minioServiceAccountSecretData struct {
	Url       string
	AccessKey string
	SecretKey string
}

func newRandomMinioServiceAccountSecretData(url string) *minioServiceAccountSecretData {
	randomName := MINIO_ACCESS_KEY_PREFIX + randomString(MINIO_ACCESS_KEY_MAX_LENGTH-len(MINIO_ACCESS_KEY_PREFIX))
	randomKey := randomString(40)
	return &minioServiceAccountSecretData{
		Url:       url,
		AccessKey: randomName,
		SecretKey: randomKey,
	}
}

func (d *minioServiceAccountSecretData) fromBase64Map(data map[string][]byte) error {
	url, err := base64.StdEncoding.DecodeString(string(data["S3_URL"]))
	if err != nil {
		return err
	}
	d.Url = string(url)

	accessKey, err := base64.StdEncoding.DecodeString(string(data["S3_ACCESS_KEY"]))
	if err != nil {
		return err
	}
	d.AccessKey = string(accessKey)

	secretKey, err := base64.StdEncoding.DecodeString(string(data["S3_SECRET_KEY"]))
	if err != nil {
		return err
	}
	d.SecretKey = string(secretKey)

	return nil
}

func (d *minioServiceAccountSecretData) toBase64Map() map[string][]byte {
	return map[string][]byte{
		"S3_URL":        []byte(base64.StdEncoding.EncodeToString([]byte(d.Url))),
		"S3_ACCESS_KEY": []byte(base64.StdEncoding.EncodeToString([]byte(d.AccessKey))),
		"S3_SECRET_KEY": []byte(base64.StdEncoding.EncodeToString([]byte(d.SecretKey))),
	}
}

type MinioServiceAccountReconcilerConfig struct {
	MinioEndpoint  string
	MinioAccessKey string
	MinioSecretKey string
}

// MinioServiceAccountReconciler reconciles a MinioServiceAccount object
type MinioServiceAccountReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	config *MinioServiceAccountReconcilerConfig
	admin  *madmin.AdminClient
}

func NewMinioServiceAccountReconciler(client client.Client, scheme *runtime.Scheme, config *MinioServiceAccountReconcilerConfig) (*MinioServiceAccountReconciler, error) {
	admin, err := madmin.New(config.MinioEndpoint, config.MinioAccessKey, config.MinioSecretKey, false)
	if err != nil {
		return nil, err
	}

	return &MinioServiceAccountReconciler{
		Client: client,
		Scheme: scheme,
		config: config,
		admin:  admin,
	}, nil
}

//+kubebuilder:rbac:groups=infra.d464.sh,resources=minioserviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infra.d464.sh,resources=minioserviceaccounts/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infra.d464.sh,resources=minioserviceaccounts/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MinioServiceAccount object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *MinioServiceAccountReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	acct := infrav1.MinioServiceAccount{}
	if err := r.Get(ctx, req.NamespacedName, &acct); err != nil {
		if errors.IsNotFound(err) {
			l.Info("MinioServiceAccount not found", "account", req.NamespacedName)
			r.garbageCollectServiceAccounts(ctx)
			return ctrl.Result{}, nil
		} else {
			l.Error(err, "unable to fetch MinioServiceAccount")
			return ctrl.Result{}, err
		}
	}
	l.Info("reconciling MinioServiceAccount", "account", acct.Name)

	secretData := &minioServiceAccountSecretData{}
	secret, err := r.getOrCreateSecretForAccount(ctx, &acct)
	if err != nil {
		l.Error(err, "unable to get or create secret for MinioServiceAccount", "account", acct.Name)
		return ctrl.Result{}, err
	}

	if err := secretData.fromBase64Map(secret.Data); err != nil {
		l.Error(err, "unable to decode secret for MinioServiceAccount", "account", acct.Name)
		return ctrl.Result{}, err
	}

	policy, err := r.policyForBuckets(acct.Spec.Buckets)
	if err != nil {
		l.Error(err, "unable to generate policy for MinioServiceAccount", "account", acct.Name)
		return ctrl.Result{}, err
	}

	if err := r.reconcileServiceAccount(ctx, secretData.AccessKey, secretData.SecretKey, policy); err != nil {
		l.Error(err, "unable to reconcile MinioServiceAccount", "account", acct.Name)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MinioServiceAccountReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.MinioServiceAccount{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}

func (r *MinioServiceAccountReconciler) reconcileServiceAccount(ctx context.Context, accessKey, secretKey string, policy []byte) error {
	l := log.FromContext(ctx)

	existingAccounts, err := r.admin.ListServiceAccounts(ctx, r.config.MinioAccessKey)
	if err != nil {
		l.Error(err, "unable to list MinioServiceAccounts")
		return err
	}

	if slices.Contains(existingAccounts.Accounts, accessKey) {
		l.Info("updating existing MinioServiceAccount", "account", accessKey)
		if err := r.admin.UpdateServiceAccount(ctx, accessKey, madmin.UpdateServiceAccountReq{
			NewPolicy:    policy,
			NewSecretKey: secretKey,
		}); err != nil {
			l.Error(err, "unable to update MinioServiceAccount", "account", accessKey)
			return err
		}
	} else {
		l.Info("creating new MinioServiceAccount", "account", accessKey)
		if _, err := r.admin.AddServiceAccount(ctx, madmin.AddServiceAccountReq{
			Policy:     policy,
			TargetUser: r.config.MinioAccessKey,
			AccessKey:  accessKey,
			SecretKey:  secretKey,
		}); err != nil {
			l.Error(err, "unable to create MinioServiceAccount", "account", accessKey)
			return err
		}
	}

	return nil
}

func (r *MinioServiceAccountReconciler) policyForBuckets(buckets []string) ([]byte, error) {
	const policyTemplate = `
{
 "Version": "2012-10-17",
 "Statement": [
  {
   "Effect": "Allow",
   "Action": [
    "s3:*"
   ],
   "Resource": $$
  }
 ]
}
`
	resources := make([]string, 0, len(buckets)*2)
	for _, bucket := range buckets {
		resources = append(resources, "arn:aws:s3:::"+bucket)
		resources = append(resources, "arn:aws:s3:::"+bucket+"/*")
	}
	serialized, err := json.Marshal(resources)
	if err != nil {
		return []byte{}, err
	}

	policy := policyTemplate
	policy = strings.ReplaceAll(policy, "$$", string(serialized))

	return []byte(policy), nil
}

func (r *MinioServiceAccountReconciler) garbageCollectServiceAccounts(ctx context.Context) {
	l := log.FromContext(ctx)
	l.Info("garbage collecting MinioServiceAccounts")

	accounts, err := r.admin.ListServiceAccounts(ctx, r.config.MinioAccessKey)
	if err != nil {
		l.Error(err, "unable to list MinioServiceAccounts")
		return
	}

	accountSet := make(map[string]struct{})
	for _, account := range accounts.Accounts {
		accountSet[account] = struct{}{}
	}

	minioAccounts := &infrav1.MinioServiceAccountList{}
	if err := r.List(ctx, minioAccounts); err != nil {
		l.Error(err, "unable to list MinioServiceAccounts")
		return
	}

	for _, minioAccount := range minioAccounts.Items {
		secret, err := r.getOrCreateSecretForAccount(ctx, &minioAccount)
		if err != nil {
			l.Error(err, "unable to get or create secret for MinioServiceAccount", "account", minioAccount.Name)
			return
		}

		secretData := &minioServiceAccountSecretData{}
		if err := secretData.fromBase64Map(secret.Data); err != nil {
			l.Error(err, "unable to decode secret for MinioServiceAccount", "account", minioAccount.Name)
			return
		}

		delete(accountSet, secretData.AccessKey)
	}

	for account := range accountSet {
		if !strings.HasPrefix(account, MINIO_ACCESS_KEY_PREFIX) {
			continue
		}

		l.Info("deleting MinioServiceAccount", "account", account)
		if err := r.admin.DeleteServiceAccount(ctx, account); err != nil {
			l.Error(err, "unable to delete MinioServiceAccount", "account", account)
		}
	}
}

func (r *MinioServiceAccountReconciler) getOrCreateSecretForAccount(ctx context.Context, acct *infrav1.MinioServiceAccount) (corev1.Secret, error) {
	l := log.FromContext(ctx)

	secret := corev1.Secret{}
	secretName := minioSecretNameForAccount(acct)

	err := r.Get(ctx, secretName, &secret)
	if !errors.IsNotFound(err) {
		return secret, err
	}

	if err == nil {
		return secret, nil
	}

	url := "http://minio.storage.svc.cluster.local:9000"
	secretData := newRandomMinioServiceAccountSecretData(url)

	secret.Name = secretName.Name
	secret.Namespace = secretName.Namespace
	secret.Data = secretData.toBase64Map()
	controllerutil.SetControllerReference(acct, &secret, r.Scheme)

	if err := r.Create(ctx, &secret); err != nil {
		l.Error(err, "unable to create secret for MinioServiceAccount", "account", acct.Name)
		return secret, err
	}

	return secret, nil
}

func minioSecretNameForAccount(acct *infrav1.MinioServiceAccount) types.NamespacedName {
	return types.NamespacedName{
		Namespace: acct.Namespace,
		Name:      acct.Name + "-minio",
	}
}

func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz" +
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}

	return string(b)
}
