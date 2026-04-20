package controllers

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	redlockv1 "github.com/example/redlock-fencing-demo/operators/redlock-operator/api/v1"
)

type RedlockClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *RedlockClusterReconciler) SetupWithManager(mgr manager.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&redlockv1.RedlockCluster{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}

func (r *RedlockClusterReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := log.FromContext(ctx)

	var cluster redlockv1.RedlockCluster
	if err := r.Get(ctx, req.NamespacedName, &cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		log.Error(err, "Unable to fetch RedlockCluster")
		return reconcile.Result{}, err
	}

	log.Info("Reconciling RedlockCluster", "name", cluster.Name, "size", cluster.Spec.Size)

	cluster.SetPhase(redlockv1.ClusterPhaseCreating)
	if err := r.Update(ctx, &cluster); err != nil {
		return reconcile.Result{}, err
	}

	if err := r.reconcileStatefulSet(ctx, &cluster); err != nil {
		return reconcile.Result{}, err
	}

	if err := r.reconcileService(ctx, &cluster); err != nil {
		return reconcile.Result{}, err
	}

	if err := r.reconcileConfigMap(ctx, &cluster); err != nil {
		return reconcile.Result{}, err
	}

	if err := r.updateStatus(ctx, &cluster); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *RedlockClusterReconciler) reconcileStatefulSet(ctx context.Context, cluster *redlockv1.RedlockCluster) error {
	log := log.FromContext(ctx)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
		},
	}

	image := cluster.Spec.Image
	if image == "" {
		image = "redis:7-alpine"
	}

	ownerRef := cluster.GetOwnerReference()

	if err := r.Get(ctx, types.NamespacedName{Name: sts.Name, Namespace: sts.Namespace}, sts); err != nil {
		if apierrors.IsNotFound(err) {
			sts.Spec = appsv1.StatefulSetSpec{
				Replicas: &cluster.Spec.Size,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app":     "redlock",
						"cluster": cluster.Name,
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app":     "redlock",
							"cluster": cluster.Name,
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "redis",
								Image: image,
								Ports: []corev1.ContainerPort{
									{ContainerPort: 6379},
								},
								Command: []string{"redis-server"},
								Args:    []string{"--appendonly", "yes"},
							},
						},
					},
				},
				ServiceName: cluster.Name + "-headless",
			}

			if cluster.Spec.Persistence.Enabled {
				sts.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "data",
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									"storage": resource.MustParse(cluster.Spec.Persistence.Size),
								},
							},
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						},
					},
				}

				sts.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
					{
						Name:      "data",
						MountPath: "/data",
					},
				}
			}

			sts.OwnerReferences = []metav1.OwnerReference{ownerRef}

			log.Info("Creating StatefulSet", "name", sts.Name)
			return r.Create(ctx, sts)
		}
		return err
	}

	if *sts.Spec.Replicas != cluster.Spec.Size {
		*sts.Spec.Replicas = cluster.Spec.Size
		log.Info("Updating StatefulSet replicas", "name", sts.Name, "replicas", cluster.Spec.Size)
		return r.Update(ctx, sts)
	}

	return nil
}

func (r *RedlockClusterReconciler) reconcileService(ctx context.Context, cluster *redlockv1.RedlockCluster) error {
	log := log.FromContext(ctx)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name + "-headless",
			Namespace: cluster.Namespace,
		},
	}

	ownerRef := cluster.GetOwnerReference()

	if err := r.Get(ctx, types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, svc); err != nil {
		if apierrors.IsNotFound(err) {
			svc.Spec = corev1.ServiceSpec{
				ClusterIP: "None",
				Selector: map[string]string{
					"app":     "redlock",
					"cluster": cluster.Name,
				},
				Ports: []corev1.ServicePort{
					{
						Name:     "redis",
						Port:     6379,
						Protocol: corev1.ProtocolTCP,
					},
				},
			}
			svc.OwnerReferences = []metav1.OwnerReference{ownerRef}

			log.Info("Creating Headless Service", "name", svc.Name)
			return r.Create(ctx, svc)
		}
		return err
	}

	return nil
}

func (r *RedlockClusterReconciler) reconcileConfigMap(ctx context.Context, cluster *redlockv1.RedlockCluster) error {
	log := log.FromContext(ctx)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name + "-config",
			Namespace: cluster.Namespace,
		},
	}

	ownerRef := cluster.GetOwnerReference()

	if err := r.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, cm); err != nil {
		if apierrors.IsNotFound(err) {
			cm.Data = map[string]string{
				"REDIS_NODES":    fmt.Sprintf("%s-0.%s-headless.%s.svc.cluster.local:6379", cluster.Name, cluster.Name, cluster.Namespace),
				"ENABLE_FENCING": fmt.Sprintf("%t", cluster.Spec.EnableFencing),
				"LOCK_TTL":       fmt.Sprintf("%d", cluster.Spec.LockTTL),
				"CLUSTER_SIZE":   fmt.Sprintf("%d", cluster.Spec.Size),
			}
			cm.OwnerReferences = []metav1.OwnerReference{ownerRef}

			log.Info("Creating ConfigMap", "name", cm.Name)
			return r.Create(ctx, cm)
		}
		return err
	}

	cm.Data["ENABLE_FENCING"] = fmt.Sprintf("%t", cluster.Spec.EnableFencing)
	cm.Data["LOCK_TTL"] = fmt.Sprintf("%d", cluster.Spec.LockTTL)

	log.Info("Updating ConfigMap", "name", cm.Name)
	return r.Update(ctx, cm)
}

func (r *RedlockClusterReconciler) updateStatus(ctx context.Context, cluster *redlockv1.RedlockCluster) error {
	log := log.FromContext(ctx)

	sts := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, sts); err != nil {
		return err
	}

	cluster.Status.CurrentReplicas = sts.Status.Replicas
	cluster.Status.ReadyReplicas = sts.Status.ReadyReplicas

	if sts.Status.ReadyReplicas == cluster.Spec.Size {
		cluster.SetPhase(redlockv1.ClusterPhaseRunning)
		cluster.Status.Leader = fmt.Sprintf("%s-0", cluster.Name)
	} else if sts.Status.ReadyReplicas > 0 && sts.Status.ReadyReplicas < cluster.Spec.Size {
		cluster.SetPhase(redlockv1.ClusterPhaseDegraded)
	}

	log.Info("Updating cluster status", "name", cluster.Name, "phase", cluster.Status.Phase, "ready", cluster.Status.ReadyReplicas)
	return r.Status().Update(ctx, cluster)
}
