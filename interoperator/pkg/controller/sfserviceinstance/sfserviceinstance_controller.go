/*
Copyright 2018 The Service Fabrik Authors.

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

package sfserviceinstance

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

	osbv1alpha1 "github.com/cloudfoundry-incubator/service-fabrik-broker/interoperator/pkg/apis/osb/v1alpha1"
	clusterFactory "github.com/cloudfoundry-incubator/service-fabrik-broker/interoperator/pkg/internal/cluster/factory"
	"github.com/cloudfoundry-incubator/service-fabrik-broker/interoperator/pkg/internal/resources"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// finalizerName is the name of the finalizer added by interoperator
const (
	finalizerName    = "interoperator.servicefabrik.io"
	errorCountKey    = "interoperator.servicefabrik.io/error"
	lastOperationKey = "interoperator.servicefabrik.io/lastoperation"
	errorThreshold   = 10
	workerCount      = 10
)

var log = logf.Log.WithName("instance.controller")

// Add creates a new SFServiceInstance Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	clusterFactory, _ := clusterFactory.New(mgr)
	return add(mgr, newReconciler(mgr, resources.New(), clusterFactory))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, resourceManager resources.ResourceManager, clusterFactory clusterFactory.ClusterFactory) reconcile.Reconciler {
	return &ReconcileSFServiceInstance{
		Client:          mgr.GetClient(),
		scheme:          mgr.GetScheme(),
		clusterFactory:  clusterFactory,
		resourceManager: resourceManager,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("sfserviceinstance-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: workerCount})
	if err != nil {
		return err
	}

	// Watch for changes to SFServiceInstance
	err = c.Watch(&source.Kind{Type: &osbv1alpha1.SFServiceInstance{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO dynamically setup rbac rules and watches
	postgres := &unstructured.Unstructured{}
	postgres.SetKind("Postgres")
	postgres.SetAPIVersion("kubedb.com/v1alpha1")
	postgres2 := &unstructured.Unstructured{}
	postgres2.SetKind("Postgresql")
	postgres2.SetAPIVersion("kubernetes.sapcloud.io/v1alpha1")
	director := &unstructured.Unstructured{}
	director.SetKind("Director")
	director.SetAPIVersion("deployment.servicefabrik.io/v1alpha1")
	docker := &unstructured.Unstructured{}
	docker.SetKind("Docker")
	docker.SetAPIVersion("deployment.servicefabrik.io/v1alpha1")
	postgresqlmts := &unstructured.Unstructured{}
	postgresqlmts.SetKind("PostgresqlMT")
	postgresqlmts.SetAPIVersion("deployment.servicefabrik.io/v1alpha1")
	vhostmts := &unstructured.Unstructured{}
	vhostmts.SetKind("VirtualHost")
	vhostmts.SetAPIVersion("deployment.servicefabrik.io/v1alpha1")
	subresources := []runtime.Object{
		&appsv1.Deployment{},
		&corev1.ConfigMap{},
		postgres,
		postgres2,
		director,
		docker,
		postgresqlmts,
		vhostmts,
	}

	for _, subresource := range subresources {
		err = c.Watch(&source.Kind{Type: subresource}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &osbv1alpha1.SFServiceInstance{},
		})
		if err != nil {
			log.Error(err, "failed to start watch")
		}
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileSFServiceInstance{}

// ReconcileSFServiceInstance reconciles a SFServiceInstance object
type ReconcileSFServiceInstance struct {
	client.Client
	scheme          *runtime.Scheme
	clusterFactory  clusterFactory.ClusterFactory
	resourceManager resources.ResourceManager
}

// Reconcile reads that state of the cluster for a SFServiceInstance object and makes changes based on the state read
// and what is in the SFServiceInstance.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=kubedb.com,resources=Postgres,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kubernetes.sapcloud.io,resources=postgresql,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=deployment.servicefabrik.io,resources=director,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=deployment.servicefabrik.io,resources=docker,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=deployment.servicefabrik.io,resources=postgresqlmt,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=deployment.servicefabrik.io,resources=virtualhost,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=,resources=configmap,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=interoperator.servicefabrik.io,resources=sfserviceinstances,verbs=get;list;watch;create;update;patch;delete
// TODO dynamically setup rbac rules and watches
func (r *ReconcileSFServiceInstance) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the ServiceInstance instance
	instance := &osbv1alpha1.SFServiceInstance{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			log.Info("instance deleted", "binding", request.NamespacedName)
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return r.handleError(instance, reconcile.Result{}, err, "", 0)
	}

	serviceID := instance.Spec.ServiceID
	planID := instance.Spec.PlanID
	instanceID := instance.GetName()
	bindingID := ""
	state := instance.GetState()
	labels := instance.GetLabels()
	lastOperation, ok := labels[lastOperationKey]
	if !ok {
		lastOperation = "in_queue"
	}

	if err := r.reconcileFinalizers(instance, 0); err != nil {
		return r.handleError(instance, reconcile.Result{Requeue: true}, nil, "", 0)
	}

	targetClient, err := r.clusterFactory.GetCluster(instanceID, bindingID, serviceID, planID)
	if err != nil {
		return r.handleError(instance, reconcile.Result{}, err, "", 0)
	}

	if state == "delete" && !instance.GetDeletionTimestamp().IsZero() {
		// The object is being deleted
		// so lets handle our external dependency
		remainingResource, err := r.resourceManager.DeleteSubResources(targetClient, instance.Status.Resources)
		if err != nil {
			log.Error(err, "Delete sub resources failed")
			return r.handleError(instance, reconcile.Result{}, err, state, 0)
		}
		err = r.setInProgress(request.NamespacedName, state, remainingResource, 0)
		if err != nil {
			return r.handleError(instance, reconcile.Result{}, err, state, 0)
		}
		lastOperation = state
	} else if state == "in_queue" || state == "update" {
		expectedResources, err := r.resourceManager.ComputeExpectedResources(r, instanceID, bindingID, serviceID, planID, osbv1alpha1.ProvisionAction, instance.GetNamespace())
		if err != nil {
			return r.handleError(instance, reconcile.Result{}, err, state, 0)
		}

		err = r.resourceManager.SetOwnerReference(instance, expectedResources, r.scheme)
		if err != nil {
			return r.handleError(instance, reconcile.Result{}, err, state, 0)
		}

		resourceRefs, err := r.resourceManager.ReconcileResources(r, targetClient, expectedResources, instance.Status.Resources)
		if err != nil {
			log.Error(err, "ReconcileResources failed")
			return r.handleError(instance, reconcile.Result{}, err, state, 0)
		}
		err = r.setInProgress(request.NamespacedName, state, resourceRefs, 0)
		if err != nil {
			return r.handleError(instance, reconcile.Result{}, err, state, 0)
		}
		lastOperation = state
	}

	err = r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		return r.handleError(instance, reconcile.Result{}, err, "", 0)
	}
	state = instance.GetState()
	labels = instance.GetLabels()
	lastOperation, ok = labels[lastOperationKey]
	if !ok {
		lastOperation = "in_queue"
	}

	if state == "in progress" {
		if lastOperation == "delete" {
			if err := r.updateDeprovisionStatus(targetClient, instance, 0); err != nil {
				return r.handleError(instance, reconcile.Result{}, err, lastOperation, 0)
			}
		} else if lastOperation == "in_queue" || lastOperation == "update" {
			err = r.updateStatus(targetClient, instance, 0)
			if err != nil {
				return r.handleError(instance, reconcile.Result{}, err, lastOperation, 0)
			}
		}
	}
	return r.handleError(instance, reconcile.Result{}, nil, lastOperation, 0)
}

func (r *ReconcileSFServiceInstance) reconcileFinalizers(object *osbv1alpha1.SFServiceInstance, retryCount int) error {
	objectID := object.GetName()
	namespace := object.GetNamespace()
	// Fetch object again before updating
	namespacedName := types.NamespacedName{
		Name:      objectID,
		Namespace: namespace,
	}
	err := r.Get(context.TODO(), namespacedName, object)
	if err != nil {
		if retryCount < errorThreshold {
			log.Info("Retrying", "function", "reconcileFinalizers", "retryCount", retryCount+1, "objectID", objectID)
			return r.reconcileFinalizers(object, retryCount+1)
		}
		log.Error(err, "failed to fetch object", "objectID", objectID)
		return err
	}
	if object.GetDeletionTimestamp().IsZero() {
		if !containsString(object.GetFinalizers(), finalizerName) {
			// The object is not being deleted, so if it does not have our finalizer,
			// then lets add the finalizer and update the object.
			object.SetFinalizers(append(object.GetFinalizers(), finalizerName))
			if err := r.Update(context.Background(), object); err != nil {
				if retryCount < errorThreshold {
					log.Info("Retrying", "function", "reconcileFinalizers", "retryCount", retryCount+1, "objectID", objectID)
					return r.reconcileFinalizers(object, retryCount+1)
				}
				log.Error(err, "failed to add finalizer", "objectID", objectID)
				return err
			}
			log.Info("added finalizer", "objectID", objectID)
		}
	}
	return nil
}

func (r *ReconcileSFServiceInstance) setInProgress(namespacedName types.NamespacedName, state string, resources []osbv1alpha1.Source, retryCount int) error {
	if state == "in_queue" || state == "update" || state == "delete" {
		instance := &osbv1alpha1.SFServiceInstance{}
		err := r.Get(context.TODO(), namespacedName, instance)
		if err != nil {
			if retryCount < errorThreshold {
				log.Info("Retrying", "function", "setInProgress", "retryCount", retryCount+1, "objectID", namespacedName.Name)
				return r.setInProgress(namespacedName, state, resources, retryCount+1)
			}
			log.Error(err, "Updating status to in progress failed")
			return err
		}
		instance.SetState("in progress")
		labels := instance.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels[lastOperationKey] = state
		instance.SetLabels(labels)
		instance.Status.Resources = resources
		err = r.Update(context.Background(), instance)
		if err != nil {
			if retryCount < errorThreshold {
				log.Info("Retrying", "function", "setInProgress", "retryCount", retryCount+1, "objectID", namespacedName.Name)
				return r.setInProgress(namespacedName, state, resources, retryCount+1)
			}
			log.Error(err, "Updating status to in progress failed")
			return err
		}
		log.Info("Updated status to in progress", "operation", state)
	}
	return nil
}

func (r *ReconcileSFServiceInstance) updateDeprovisionStatus(targetClient client.Client, instance *osbv1alpha1.SFServiceInstance, retryCount int) error {
	serviceID := instance.Spec.ServiceID
	planID := instance.Spec.PlanID
	instanceID := instance.GetName()
	bindingID := ""
	namespace := instance.GetNamespace()
	computedStatus, err := r.resourceManager.ComputeStatus(r, targetClient, instanceID, bindingID, serviceID, planID, osbv1alpha1.ProvisionAction, namespace)
	if err != nil {
		log.Error(err, "ComputeStatus failed for deprovision")
		return err
	}

	// Fetch object again before updating status
	namespacedName := types.NamespacedName{
		Name:      instanceID,
		Namespace: namespace,
	}
	err = r.Get(context.TODO(), namespacedName, instance)
	if err != nil {
		log.Error(err, "Failed to get instance")
		return err
	}

	updateRequired := false
	updatedStatus := instance.Status.DeepCopy()
	updatedStatus.State = computedStatus.Deprovision.State
	updatedStatus.Error = computedStatus.Deprovision.Error
	updatedStatus.Description = computedStatus.Deprovision.Response

	remainingResource := []osbv1alpha1.Source{}
	for _, subResource := range instance.Status.Resources {
		resource := &unstructured.Unstructured{}
		resource.SetKind(subResource.Kind)
		resource.SetAPIVersion(subResource.APIVersion)
		resource.SetName(subResource.Name)
		resource.SetNamespace(subResource.Namespace)
		namespacedName := types.NamespacedName{
			Name:      resource.GetName(),
			Namespace: resource.GetNamespace(),
		}
		err := targetClient.Get(context.TODO(), namespacedName, resource)
		if !errors.IsNotFound(err) {
			remainingResource = append(remainingResource, subResource)
		}
	}
	updatedStatus.Resources = remainingResource
	if !reflect.DeepEqual(&instance.Status, updatedStatus) {
		updatedStatus.DeepCopyInto(&instance.Status)
		updateRequired = true
	}

	if instance.GetState() == "succeeded" || len(remainingResource) == 0 {
		// remove our finalizer from the list and update it.
		log.Info("Removing finalizer", "instance", instanceID)
		instance.SetFinalizers(removeString(instance.GetFinalizers(), finalizerName))
		instance.SetState("succeeded")
		updateRequired = true
	}

	if updateRequired {
		log.Info("Updating deprovision status from template", "instance", namespacedName)
		if err := r.Update(context.Background(), instance); err != nil {
			if retryCount < errorThreshold {
				log.Info("Retrying", "function", "updateDeprovisionStatus", "retryCount", retryCount+1, "instanceID", instanceID)
				return r.updateDeprovisionStatus(targetClient, instance, retryCount+1)
			}
			log.Error(err, "failed to update deprovision status", "instance", instanceID)
			return err
		}
	}
	return nil
}

func (r *ReconcileSFServiceInstance) updateStatus(targetClient client.Client, instance *osbv1alpha1.SFServiceInstance, retryCount int) error {
	serviceID := instance.Spec.ServiceID
	planID := instance.Spec.PlanID
	instanceID := instance.GetName()
	bindingID := ""
	namespace := instance.GetNamespace()
	computedStatus, err := r.resourceManager.ComputeStatus(r, targetClient, instanceID, bindingID, serviceID, planID, osbv1alpha1.ProvisionAction, namespace)
	if err != nil {
		log.Error(err, "Compute status failed", "instance", instanceID)
		return err
	}

	// Fetch object again before updating status
	namespacedName := types.NamespacedName{
		Name:      instanceID,
		Namespace: namespace,
	}
	err = r.Get(context.TODO(), namespacedName, instance)
	if err != nil {
		log.Error(err, "failed to fetch instance", "instance", instanceID)
		return err
	}
	updatedStatus := instance.Status.DeepCopy()
	updatedStatus.State = computedStatus.Provision.State
	updatedStatus.Error = computedStatus.Provision.Error
	updatedStatus.Description = computedStatus.Provision.Response
	updatedStatus.DashboardURL = computedStatus.Provision.DashboardURL

	if !reflect.DeepEqual(&instance.Status, updatedStatus) {
		updatedStatus.DeepCopyInto(&instance.Status)
		log.Info("Updating provision status from template", "instance", namespacedName)
		err = r.Update(context.Background(), instance)
		if err != nil {
			if retryCount < errorThreshold {
				log.Info("Retrying", "function", "updateStatus", "retryCount", retryCount+1, "instanceID", instanceID)
				return r.updateStatus(targetClient, instance, retryCount+1)
			}
			log.Error(err, "failed to update status")
			return err
		}
	}
	return nil
}

//
// Helper functions to check and remove string from a slice of strings.
//
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}

func (r *ReconcileSFServiceInstance) handleError(object *osbv1alpha1.SFServiceInstance, result reconcile.Result, inputErr error, lastOperation string, retryCount int) (reconcile.Result, error) {
	objectID := object.GetName()
	namespace := object.GetNamespace()
	// Fetch object again before updating
	namespacedName := types.NamespacedName{
		Name:      objectID,
		Namespace: namespace,
	}
	err := r.Get(context.TODO(), namespacedName, object)
	if err != nil {
		if errors.IsNotFound(err) {
			return result, inputErr
		}
		if retryCount < errorThreshold {
			log.Info("Retrying", "function", "handleError", "retryCount", retryCount+1, "lastOperation", lastOperation, "err", inputErr, "objectID", objectID)
			return r.handleError(object, result, inputErr, lastOperation, retryCount+1)
		}
		log.Error(err, "failed to fetch object", "objectID", objectID)
		return result, inputErr
	}

	labels := object.GetLabels()
	var count int64

	if labels == nil {
		labels = make(map[string]string)
	}

	countString, ok := labels[errorCountKey]
	if !ok {
		count = 0
	} else {
		i, err := strconv.ParseInt(countString, 10, 64)
		if err != nil {
			count = 0
		} else {
			count = i
		}
	}

	if inputErr == nil {
		if count == 0 {
			//No change for count
			return result, inputErr
		}
		count = 0
	} else {
		count++
	}

	if count > errorThreshold {
		log.Error(inputErr, "Retry threshold reached. Ignoring error", "objectID", objectID)
		object.Status.State = "failed"
		object.Status.Error = fmt.Sprintf("Retry threshold reached for %s.\n%s", objectID, inputErr.Error())
		object.Status.Description = "Service Broker Error, status code: ETIMEDOUT, error code: 10008"
		if lastOperation != "" {
			labels[lastOperationKey] = lastOperation
			object.SetLabels(labels)
		}
		err := r.Update(context.TODO(), object)
		if err != nil {
			if retryCount < errorThreshold {
				log.Info("Retrying", "function", "handleError", "retryCount", retryCount+1, "lastOperation", lastOperation, "err", inputErr, "objectID", objectID)
				return r.handleError(object, result, inputErr, lastOperation, retryCount+1)
			}
			log.Error(err, "Failed to set state to failed", "objectID", objectID)
		}
		return result, nil
	}

	labels[errorCountKey] = strconv.FormatInt(count, 10)
	object.SetLabels(labels)
	err = r.Update(context.TODO(), object)
	if err != nil {
		if retryCount < errorThreshold {
			log.Info("Retrying", "function", "handleError", "retryCount", retryCount+1, "lastOperation", lastOperation, "err", inputErr, "objectID", objectID)
			return r.handleError(object, result, inputErr, lastOperation, retryCount+1)
		}
		log.Error(err, "Failed to update error count label", "objectID", objectID, "count", count)
	}
	return result, inputErr
}
