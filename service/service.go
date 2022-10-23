package service

import (
	"context"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	stdlog "log"

	v1 "k8s.io/api/core/v1"
	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	redisv1 "redis-demo/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func CreatePod(client client.Client, obj *redisv1.Redis, podName string, schema *runtime.Scheme) (*string, error) {
	if PodIsNotAlreadyExist(client, obj.Namespace, podName) {
		return nil, nil
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: obj.Namespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:            podName,
					Image:           "redis:5-alpine",
					ImagePullPolicy: v1.PullIfNotPresent,
					Ports: []v1.ContainerPort{
						{
							ContainerPort: int32(obj.Spec.Port),
						},
					},
				},
			},
		},
	}

	// set ownerReferences
	if err := controllerutil.SetControllerReference(obj, pod, schema); err != nil {
		return nil, err
	}

	if err := client.Create(context.Background(), pod); err != nil {
		return nil, err
	}
	stdlog.Println("创建新的Pod资源:", pod.Name)

	return &podName, nil
}

// GenPodName 生成 Pod Name
// example: ["myhedui-0", "myhedui-1" ... ]
func GenPodName(prefix string, replicas int) []string {
	ans := make([]string, 0)
	for i := 0; i < replicas; i++ {
		ans = append(ans, fmt.Sprintf("%s-%d", prefix, i))
	}
	return ans
}

// RebuildPod 重建被删除的 Pod
func RebuildPod(evt event.DeleteEvent, ltf workqueue.RateLimitingInterface) {
	stdlog.Println("检测到有Pod删除, Pod名称：", evt.Object.GetName())

	for _, ref := range evt.Object.GetOwnerReferences() {
		if ref.Kind == "Redis" && ref.APIVersion == "redis.hedui.com/v1" {
			ltf.Add(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      ref.Name,
					Namespace: evt.Object.GetNamespace(),
				},
			})
		}
	}
}

// PodIsNotAlreadyExist 检测 Pod 是否已经创建
func PodIsNotAlreadyExist(client client.Client, podNamespace, podName string) bool {
	if err := client.Get(context.Background(), types.NamespacedName{
		Namespace: podNamespace,
		Name:      podName,
	}, &v1.Pod{}); err != nil {
		return false
	}

	return true
}

func Delete(client client.Client, obj *redisv1.Redis) error {
	stdlog.Println("检测到删除资源，正在清理Pod、Finalizers...")
	for _, name := range obj.Finalizers {
		if err := client.Delete(context.Background(), &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: obj.Namespace,
			},
		}); err != nil {
			return err
		}
	}

	obj.Finalizers = []string{}
	stdlog.Println("检测到删除资源，资源清理完毕...")
	return client.Update(context.Background(), obj)
}
