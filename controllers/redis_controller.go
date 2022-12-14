/*
Copyright 2022.

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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	stdlog "log"
	"redis-demo/service"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	redisv1 "redis-demo/api/v1"
)

// RedisReconciler reconciles a Redis object
type RedisReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Event  record.EventRecorder
}

//+kubebuilder:rbac:groups=redis.hedui.com,resources=redis,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=redis.hedui.com,resources=redis/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=redis.hedui.com,resources=redis/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Redis object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *RedisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var (
		isUpdate = false
	)

	obj := &redisv1.Redis{}
	if err := r.Get(context.Background(), req.NamespacedName, obj); err != nil {
		stdlog.Println("???????????????????????????????????????, err:", err.Error())
		return ctrl.Result{}, nil
	}

	if !obj.DeletionTimestamp.IsZero() {
		r.Event.Event(obj, v1.EventTypeNormal, "Deleted", "????????????")
		return ctrl.Result{}, service.Delete(r.Client, obj)
	}

	podNameList := service.GenPodName(obj.Name, obj.Spec.Replicas)
	for _, name := range podNameList {
		podNameRes, err := service.CreatePod(r.Client, obj, name, r.Scheme)
		if err != nil {
			return ctrl.Result{}, err
		}

		if podNameRes != nil {
			if controllerutil.ContainsFinalizer(obj, *podNameRes) {
				continue
			}

			obj.Finalizers = append(obj.Finalizers, *podNameRes)
			isUpdate = true
		}
	}

	// ????????? apply ?????????????????????????????????????????????????????????
	if len(podNameList) < len(obj.Finalizers) {
		stdlog.Println("??????????????????????????????.")
		for i := len(podNameList); i < len(obj.Finalizers); i++ {
			if err := r.Client.Delete(context.Background(), &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      obj.Finalizers[i],
					Namespace: obj.Namespace,
				},
			}, &client.DeleteOptions{}); err != nil {
				stdlog.Println("?????????????????????, err:", err.Error())
				return ctrl.Result{}, err
			}
		}

		obj.Finalizers = podNameList
		isUpdate = true
		stdlog.Println("?????????????????????.")
		r.Event.Event(obj, v1.EventTypeNormal, "Replicas", "????????????")
	}

	if isUpdate {
		stdlog.Println("?????????????????????:", obj.Finalizers)
		if err := r.Update(context.Background(), obj); err != nil {
			stdlog.Println(err)
			return ctrl.Result{}, err
		}
		r.Event.Event(obj, v1.EventTypeNormal, "Updated", "????????????")
		obj.Status.Replicas = len(obj.Finalizers)
		if err := r.Status().Update(context.Background(), obj, &client.UpdateOptions{}); err != nil {
			stdlog.Println("?????? status ??????, err: ", err.Error())
			return ctrl.Result{}, nil
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
// ?????????????????????Pod
func (r *RedisReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&redisv1.Redis{}).
		Watches(
			&source.Kind{Type: &v1.Pod{}},
			handler.Funcs{
				DeleteFunc: service.RebuildPod,
			},
		).
		Complete(r)
}
