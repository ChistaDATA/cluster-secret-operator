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

package controllers

import (
	"context"
	"fmt"
	v12 "github.com/ChistaDATA/cluster-secret-operator/api/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"strings"
	"time"
)

// Definitions to manage status conditions
const (
	// typeAvailableClusterSecret represents the status of the Deployment reconciliation
	typeAvailableClusterSecret = "Available"
)

// ClusterSecretReconciler reconciles a ClusterSecret object
type ClusterSecretReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups=chistadata.io,resources=clustersecrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=chistadata.io,resources=clustersecrets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=chistadata.io,resources=clustersecrets/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *ClusterSecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	clusterSecret := &v12.ClusterSecret{}
	err := r.Get(ctx, req.NamespacedName, clusterSecret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("ClusterSecret resource not found. Ignoring since object must be deleted", "clusterSecret.Name", clusterSecret.Name)
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get ClusterSecret, requeuing the request", "clusterSecret.Name", clusterSecret.Name)
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	if clusterSecret.Status.Conditions == nil || len(clusterSecret.Status.Conditions) == 0 {
		meta.SetStatusCondition(&clusterSecret.Status.Conditions, metav1.Condition{Type: typeAvailableClusterSecret, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
		if err = r.Status().Update(ctx, clusterSecret); err != nil {
			log.Error(err, "Failed to update ClusterSecret status", "clusterSecret.Name", clusterSecret.Name)
			return ctrl.Result{}, err
		}

		if err := r.Get(ctx, req.NamespacedName, clusterSecret); err != nil {
			log.Error(err, "Failed to re-fetch ClusterSecret", "clusterSecret.Name", clusterSecret.Name)
			return ctrl.Result{}, err
		}
	}

	isMarkedToBeDeleted := clusterSecret.GetDeletionTimestamp() != nil
	if isMarkedToBeDeleted {
		return ctrl.Result{}, nil
	}

	ls := getLabels(clusterSecret.Name)

	k8sNamespaceList := &v1.NamespaceList{}
	err = r.List(ctx, k8sNamespaceList)
	if err != nil {
		return r.updateClusterSecretStatus(ctx, metav1.ConditionFalse, clusterSecret, fmt.Sprintf("Failed to get namespace list: (%s)", err), err)
	}
	namespaceList, err := FilterNamespaceList(k8sNamespaceList, clusterSecret)
	if err != nil {
		return r.updateClusterSecretStatus(ctx, metav1.ConditionFalse, clusterSecret, fmt.Sprintf("Failed to filter namespace list (%s)", err), err)
	}

	if clusterSecret.Spec.ValueFrom.SecretName == "" {
		if clusterSecret.Spec.Data == nil {
			msg := "the Data must be provided when the ValueFrom is not provided"
			log.Error(err, msg, "clusterSecret.Name", clusterSecret.Name)
			return r.updateClusterSecretStatus(ctx, metav1.ConditionFalse, clusterSecret, msg, err)
		}
		_, err = r.createNewSecrets(ctx, clusterSecret, ls, namespaceList)
		if err != nil {
			return r.updateClusterSecretStatus(ctx, metav1.ConditionFalse, clusterSecret, fmt.Sprintf("Failed to create Secret: (%s)", err), err)
		}
	} else {
		source := &v1.Secret{}
		err = r.Get(ctx, types.NamespacedName{
			Name:      clusterSecret.Spec.ValueFrom.SecretName,
			Namespace: clusterSecret.Spec.ValueFrom.SecretNamespace,
		}, source)
		if err != nil {
			if apierrors.IsNotFound(err) {
				msg := "the secret in the ValueFrom could not be found"
				log.Error(err, msg, "clusterSecret.Name", clusterSecret.Name)
				return r.updateClusterSecretStatus(ctx, metav1.ConditionFalse, clusterSecret, msg, err)
			}
			return ctrl.Result{}, err
		}
		clusterSecret.Spec.Type = source.Type
		clusterSecret.Spec.Data = source.Data
		_, err = r.createNewSecrets(ctx, clusterSecret, ls, namespaceList)
		if err != nil {
			return r.updateClusterSecretStatus(ctx, metav1.ConditionTrue, clusterSecret, fmt.Sprintf("Failed to create Secret: (%s)", err), err)
		}
	}

	return r.updateClusterSecretStatus(ctx, metav1.ConditionFalse, clusterSecret, "Secrets creation has finished successfully", nil)
}

func (r *ClusterSecretReconciler) updateClusterSecretStatus(ctx context.Context, status metav1.ConditionStatus, clusterSecret *v12.ClusterSecret, msg string, err error) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	meta.SetStatusCondition(&clusterSecret.Status.Conditions, metav1.Condition{Type: typeAvailableClusterSecret,
		Status: status, Reason: "Reconciling",
		Message: msg})
	if err := r.Status().Update(ctx, clusterSecret); err != nil {
		log.Error(err, "Failed to update ClusterSecret status", "clusterSecret.Name", clusterSecret.Name)
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, err
}

func (r *ClusterSecretReconciler) createNewSecrets(ctx context.Context, clusterSecret *v12.ClusterSecret, ls map[string]string, namespaceList []string) ([]*v1.Secret, error) {
	log := ctrllog.FromContext(ctx)

	var secretList []*v1.Secret
	log.Info("Processing the cluster secret", "clusterSecret.Namespace", clusterSecret.Namespace, "clusterSecret.Name", clusterSecret.Name)
	for _, n := range namespaceList {
		log.V(2).Info("Checking if the secret exists", "secret.Namespace", n, "secret.Name", clusterSecret.Name)
		found := &v1.Secret{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      clusterSecret.Name,
			Namespace: n,
		}, found)
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Info("Secret not found, creating a new secret", "secret.Namespace", n, "secret.Name", clusterSecret.Name)
				newSecret := &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      clusterSecret.Name,
						Namespace: n,
						Labels:    ls,
					},
					Type: clusterSecret.Spec.Type,
					Data: clusterSecret.Spec.Data,
				}
				err = r.Create(ctx, newSecret)
				if err != nil {
					log.Error(err, err.Error())
					return nil, err
				}
				secretList = append(secretList, newSecret)
				continue
			}
			log.Error(err, "Error occurred when querying the secret", "secret.Namespace", n, "secret.Name", clusterSecret.Name)
			return nil, err
		} else {
			if found.Annotations["chistadata.io/name"] != "" {
				err = fmt.Errorf("the secret %s/%s has the 'chistadata.io/name' annotation. This might be a cyclic reference, skipping the update", found.Namespace, found.Name)
				return nil, err
			}
			if reflect.DeepEqual(clusterSecret.Spec.Data, found.Data) && clusterSecret.Spec.Type == found.Type {
				log.V(2).Info("The secret value has not changed", "secret.Namespace", found.Namespace, "secret.Name", found.Name)
				continue
			}
			log.Info("Secret found, updating its values", "secret.Namespace", n, "secret.Name", clusterSecret.Name)
			found.Type = clusterSecret.Spec.Type
			found.Data = clusterSecret.Spec.Data
			err = r.Update(ctx, found)
			if err != nil {
				log.Error(err, err.Error())
				return nil, err
			}
			log.Info("Secret updated successfully", "secret.Namespace", n, "secret.Name", clusterSecret.Name)
			secretList = append(secretList, found)
		}
	}
	return secretList, nil
}

func FilterNamespaceList(k8sNamespaceList *v1.NamespaceList, clusterSecret *v12.ClusterSecret) ([]string, error) {
	// TODO: Improve this filtering logic
	var namespaceList []string
	for _, namespace := range clusterSecret.Spec.IncludeNamespaces {
		if namespace == "*" {
			for _, k8sNamespace := range k8sNamespaceList.Items {
				namespaceList = append(namespaceList, k8sNamespace.Name)
			}
			break
		} else if strings.HasPrefix(namespace, "*") {
			for _, k8sNamespace := range k8sNamespaceList.Items {
				if strings.HasSuffix(k8sNamespace.Name, namespace[1:]) {
					namespaceList = append(namespaceList, k8sNamespace.Name)
				}
			}
		} else if strings.HasSuffix(namespace, "*") {
			for _, k8sNamespace := range k8sNamespaceList.Items {
				if strings.HasSuffix(k8sNamespace.Name, namespace[:1]) {
					namespaceList = append(namespaceList, k8sNamespace.Name)
				}
			}
		} else {
			for _, k8sNamespace := range k8sNamespaceList.Items {
				if k8sNamespace.Name == namespace {
					namespaceList = append(namespaceList, k8sNamespace.Name)
				}
			}
		}
	}
	for _, namespace := range clusterSecret.Spec.ExcludeNamespaces {
		if namespace == "*" {
			return []string{}, nil
		} else if strings.HasPrefix(namespace, "*") {
			for i := 0; i < len(namespaceList); i++ {
				if strings.HasSuffix(namespaceList[i], namespace[1:]) {
					namespaceList[i] = namespaceList[len(namespaceList)-1]
					namespaceList = namespaceList[:len(namespaceList)-1]
					i--
				}
			}
		} else if strings.HasSuffix(namespace, "*") {
			for i := 0; i < len(namespaceList); i++ {
				if strings.HasPrefix(namespaceList[i], namespace[:1]) {
					namespaceList[i] = namespaceList[len(namespaceList)-1]
					namespaceList = namespaceList[:len(namespaceList)-1]
					i--
				}
			}
		} else {
			for i := 0; i < len(namespaceList); i++ {
				if namespaceList[i] == namespace {
					namespaceList[i] = namespaceList[len(namespaceList)-1]
					namespaceList = namespaceList[:len(namespaceList)-1]
					i--
				}
			}
		}
	}
	return namespaceList, nil
}

func getLabels(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/cluster-secret": name,
		"app.kubernetes.io/part-of":        "cluster-secret-operator",
		"app.kubernetes.io/created-by":     "controller-manager",
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v12.ClusterSecret{}).
		WithEventFilter(predicate.Funcs{
			UpdateFunc: func(event event.UpdateEvent) bool {
				return event.ObjectOld.GetGeneration() != event.ObjectNew.GetGeneration() ||
					event.ObjectOld.GetAnnotations()["chistadata.io/new-namespace-created-at"] != event.ObjectNew.GetAnnotations()["chistadata.io/new-namespace-created-at"]
			}}).
		Complete(r)
}
