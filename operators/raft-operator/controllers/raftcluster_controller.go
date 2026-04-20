package controllers

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/api/resource"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	raftv1 "github.com/example/redlock-fencing-demo/operators/raft-operator/api/v1"
)

type RaftClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *RaftClusterReconciler) SetupWithManager(mgr manager.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&raftv1.RaftCluster{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}

func (r *RaftClusterReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := log.FromContext(ctx)

	var cluster raftv1.RaftCluster
	if err := r.Get(ctx, req.NamespacedName, &cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		log.Error(err, "Unable to fetch RaftCluster")
		return reconcile.Result{}, err
	}

	log.Info("Reconciling RaftCluster", "name", cluster.Name, "size", cluster.Spec.Size)

	cluster.SetPhase(raftv1.RaftClusterPhaseCreating)
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

func (r *RaftClusterReconciler) reconcileStatefulSet(ctx context.Context, cluster *raftv1.RaftCluster) error {
	log := log.FromContext(ctx)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
		},
	}

	image := cluster.Spec.Image
	if image == "" {
		image = "etcd:3.5"
	}

	electionTimeout := cluster.Spec.ElectionTimeout
	if electionTimeout == 0 {
		electionTimeout = 150
	}

	heartbeatInterval := cluster.Spec.HeartbeatInterval
	if heartbeatInterval == 0 {
		heartbeatInterval = 100
	}

	ownerRef := cluster.GetOwnerReference()

	if err := r.Get(ctx, types.NamespacedName{Name: sts.Name, Namespace: sts.Namespace}, sts); err != nil {
		if apierrors.IsNotFound(err) {
			sts.Spec = appsv1.StatefulSetSpec{
				Replicas: &cluster.Spec.Size,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app":     "raft",
						"cluster": cluster.Name,
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app":     "raft",
							"cluster": cluster.Name,
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "etcd",
								Image: image,
								Ports: []corev1.ContainerPort{
									{ContainerPort: 2379, Name: "client"},
									{ContainerPort: 2380, Name: "peer"},
								},
								Command: []string{"/usr/local/bin/etcd"},
								Args: []string{
									"--name=$(MY_POD_NAME)",
									"--initial-advertise-peer-urls=http://$(MY_POD_NAME).$(MY_POD_SERVICE_NAME).$(MY_POD_NAMESPACE).svc.cluster.local:2380",
									"--listen-peer-urls=http://0.0.0.0:2380",
									"--listen-client-urls=http://0.0.0.0:2379",
									"--advertise-client-urls=http://$(MY_POD_NAME).$(MY_POD_SERVICE_NAME).$(MY_POD_NAMESPACE).svc.cluster.local:2379",
									fmt.Sprintf("--election-timeout=%d", electionTimeout),
									fmt.Sprintf("--heartbeat-interval=%d", heartbeatInterval),
								},
								Env: []corev1.EnvVar{
									{Name: "MY_POD_NAME", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}}},
									{Name: "MY_POD_SERVICE_NAME", Value: cluster.Name + "-headless"},
									{Name: "MY_POD_NAMESPACE", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"}}},
								},
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
							Resources: corev1.ResourceRequirements{
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
						MountPath: "/var/lib/etcd",
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

func (r *RaftClusterReconciler) reconcileService(ctx context.Context, cluster *raftv1.RaftCluster) error {
	log := log.FromContext(ctx)

	headlessSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name + "-headless",
			Namespace: cluster.Namespace,
		},
	}

	ownerRef := cluster.GetOwnerReference()

	if err := r.Get(ctx, types.NamespacedName{Name: headlessSvc.Name, Namespace: headlessSvc.Namespace}, headlessSvc); err != nil {
		if apierrors.IsNotFound(err) {
			headlessSvc.Spec = corev1.ServiceSpec{
				ClusterIP: "None",
				Selector: map[string]string{
					"app":     "raft",
					"cluster": cluster.Name,
				},
				Ports: []corev1.ServicePort{
					{Name: "client", Port: 2379},
					{Name: "peer", Port: 2380},
				},
			}
			headlessSvc.OwnerReferences = []metav1.OwnerReference{ownerRef}

			log.Info("Creating Headless Service", "name", headlessSvc.Name)
			return r.Create(ctx, headlessSvc)
		}
		return err
	}

	clientSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name + "-client",
			Namespace: cluster.Namespace,
		},
	}

	if err := r.Get(ctx, types.NamespacedName{Name: clientSvc.Name, Namespace: clientSvc.Namespace}, clientSvc); err != nil {
		if apierrors.IsNotFound(err) {
			clientSvc.Spec = corev1.ServiceSpec{
				Selector: map[string]string{
					"app":     "raft",
					"cluster": cluster.Name,
				},
				Ports: []corev1.ServicePort{
					{Name: "client", Port: 2379},
				},
			}
			clientSvc.OwnerReferences = []metav1.OwnerReference{ownerRef}

			log.Info("Creating Client Service", "name", clientSvc.Name)
			return r.Create(ctx, clientSvc)
		}
		return err
	}

	return nil
}

func (r *RaftClusterReconciler) reconcileConfigMap(ctx context.Context, cluster *raftv1.RaftCluster) error {
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
				"CLUSTER_SIZE":        fmt.Sprintf("%d", cluster.Spec.Size),
				"ELECTION_TIMEOUT":    fmt.Sprintf("%d", cluster.Spec.ElectionTimeout),
				"HEARTBEAT_INTERVAL":  fmt.Sprintf("%d", cluster.Spec.HeartbeatInterval),
				"SERVICE_NAME":        cluster.Name + "-headless",
			}
			cm.OwnerReferences = []metav1.OwnerReference{ownerRef}

			log.Info("Creating ConfigMap", "name", cm.Name)
			return r.Create(ctx, cm)
		}
		return err
	}

	return nil
}

func (r *RaftClusterReconciler) updateStatus(ctx context.Context, cluster *raftv1.RaftCluster) error {
	log := log.FromContext(ctx)

	sts := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, sts); err != nil {
		return err
	}

	cluster.Status.CurrentReplicas = sts.Status.Replicas
	cluster.Status.ReadyReplicas = sts.Status.ReadyReplicas

	if sts.Status.ReadyReplicas == cluster.Spec.Size {
		cluster.SetPhase(raftv1.RaftClusterPhaseRunning)
		cluster.Status.Leader = fmt.Sprintf("%s-0", cluster.Name)
	} else if sts.Status.ReadyReplicas > 0 && sts.Status.ReadyReplicas < cluster.Spec.Size {
		cluster.SetPhase(raftv1.RaftClusterPhaseDegraded)
	}

	log.Info("Updating cluster status", "name", cluster.Name, "phase", cluster.Status.Phase, "ready", cluster.Status.ReadyReplicas)
	return r.Status().Update(ctx, cluster)
}
