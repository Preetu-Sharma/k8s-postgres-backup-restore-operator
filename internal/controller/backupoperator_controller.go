/*
Copyright 2026.

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

package controller

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/runtime"
	types "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	event "sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	appsv1alpha1 "github.com/dev/backupOperator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BackupOperatorReconciler reconciles a BackupOperator object
type BackupOperatorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=apps.dev.com,resources=backupoperators,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.dev.com,resources=backupoperators/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.dev.com,resources=backupoperators/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the BackupOperator object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/reconcile
func (r *BackupOperatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Get the BackupOperator instance
	var backupOperator appsv1alpha1.BackupOperator
	if err := r.Get(ctx, req.NamespacedName, &backupOperator); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return. Created objects are automatically garbage collected.
			log.Info("BackupOperator resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// Ensure cronjob exists and matches the BackupOperator spec
	var cronJob batchv1.CronJob
	err := r.Get(ctx, types.NamespacedName{Name: backupOperator.Name, Namespace: backupOperator.Namespace}, &cronJob)
	if err != nil && apierrors.IsNotFound(err) {
		// CronJob not found, create it
		newCronJob := r.constructCronJob(&backupOperator)

		if err := r.Create(ctx, newCronJob); err != nil {
			log.Error(err, "Failed to create CronJob for BackupOperator", "BackupOperator", backupOperator.Name)
			return ctrl.Result{}, err
		}

		log.Info("Created CronJob for BackupOperator", "BackupOperator", backupOperator.Name)
		return ctrl.Result{}, nil

	} else if err != nil {
		log.Error(err, "Failed to get CronJob for BackupOperator", "BackupOperator", backupOperator.Name)
		return ctrl.Result{}, err

	} else {
		// CronJob exists, check if it needs to be updated
		if !r.cronJobMatchesSpec(&backupOperator, &cronJob) {
			updatedCronJob := r.constructCronJob(&backupOperator)
			cronJob.Spec = updatedCronJob.Spec

			if err := r.Update(ctx, &cronJob); err != nil {
				log.Error(err, "Failed to update CronJob for BackupOperator", "BackupOperator", backupOperator.Name)
				return ctrl.Result{}, err
			}

			log.Info("Updated CronJob for BackupOperator", "BackupOperator", backupOperator.Name)
			return ctrl.Result{}, nil
		}
	}

	// compare backup jobs and update the backupoperator status
	var jobList batchv1.JobList
	if err := r.List(ctx, &jobList, client.InNamespace(backupOperator.Namespace)); err != nil {
		log.Error(err, "Failed to list Jobs for BackupOperator", "BackupOperator", backupOperator.Name)
		return ctrl.Result{}, err
	}

	// Update BackupOperator status based on job status

	for _, job := range jobList.Items {

		if !metav1.IsControlledBy(&job, &backupOperator) {
			continue
		}

		jobStatus := CheckJobStatus(&job)
		log.Info("Job status", "Job", job.Name, "Status", jobStatus.Phase)

		switch jobStatus.Phase {
		case "Complete":
			backupOperator.Status.LastBackupTime = metav1.Now()
			backupOperator.Status.LastBackupStatus = "Success"
			backupOperator.Status.ActiveBackupJobs = removeFromSlice(backupOperator.Status.ActiveBackupJobs, job.Name)
			meta.SetStatusCondition(&backupOperator.Status.Conditions, metav1.Condition{
				Type:    "Ready",
				Status:  metav1.ConditionTrue,
				Reason:  "BackupSuccessful",
				Message: fmt.Sprintf("Backup job %s completed successfully", job.Name),
			})
		case "Failed":
			backupOperator.Status.LastBackupTime = metav1.Now()
			backupOperator.Status.LastBackupStatus = "Failed"
			backupOperator.Status.ActiveBackupJobs = removeFromSlice(backupOperator.Status.ActiveBackupJobs, job.Name)
			meta.SetStatusCondition(&backupOperator.Status.Conditions, metav1.Condition{
				Type:    "Ready",
				Status:  metav1.ConditionFalse,
				Reason:  "BackupFailed",
				Message: fmt.Sprintf("Backup job %s failed", job.Name),
			})
		case "Active":
			if !contains(backupOperator.Status.ActiveBackupJobs, job.Name) {
				backupOperator.Status.ActiveBackupJobs = append(backupOperator.Status.ActiveBackupJobs, job.Name)
			}
			meta.SetStatusCondition(&backupOperator.Status.Conditions, metav1.Condition{
				Type:    "Ready",
				Status:  metav1.ConditionFalse,
				Reason:  "BackupInProgress",
				Message: fmt.Sprintf("Backup job %s is in progress", job.Name),
			})
		default:
			// For other statuses (Pending, Suspended, Unknown), we can choose to log or ignore
			log.Info("Job in non-terminal state", "Job", job.Name, "Status", jobStatus.Phase)
		}
	}

	if err := r.Status().Update(ctx, &backupOperator); err != nil {
		log.Error(err, "Failed to update BackupOperator status", "BackupOperator", backupOperator.Name)
		return ctrl.Result{}, err
	}

	// integrate cleanup cronjob
	var cleanupCronJob batchv1.CronJob
	cleanupCronJobKey := types.NamespacedName{
		Name:      backupOperator.Name + "-cleanup",
		Namespace: backupOperator.Namespace,
	}
	err = r.Get(ctx, cleanupCronJobKey, &cleanupCronJob)
	if err != nil && apierrors.IsNotFound(err) {
		// Cleanup CronJob not found, create it
		newCleanupCronJob := r.constructCleanupCronJob(&backupOperator)

		if err := r.Create(ctx, newCleanupCronJob); err != nil {
			log.Error(err, "Failed to create Cleanup CronJob for BackupOperator", "BackupOperator", backupOperator.Name)
			return ctrl.Result{}, err
		}

		log.Info("Created Cleanup CronJob for BackupOperator", "BackupOperator", backupOperator.Name)
	} else if err != nil {
		log.Error(err, "Failed to get Cleanup CronJob for BackupOperator", "BackupOperator", backupOperator.Name)
		return ctrl.Result{}, err
	}

	log.Info(
		"Restore block entered",
		"restore", backupOperator.Spec.RestoreGeneration,
		"restoreStatus", backupOperator.Status.RestoreStatus,
	)

	// Re-fetch before restore block to avoid resourceVersion conflict
	// with the status update that happened after the job loop
	if err := r.Get(ctx, req.NamespacedName, &backupOperator); err != nil {
		log.Error(err, "Failed to re-fetch BackupOperator before restore block")
		return ctrl.Result{}, err
	}

	// integrate restore job if Restore field is set to true
	if backupOperator.Spec.RestoreGeneration > backupOperator.Status.RestoreStatus.ObservedRestoreGeneration {
		log.Info(
			"New restore request detected",
			"requestedGeneration", backupOperator.Spec.RestoreGeneration,
			"observedGeneration", backupOperator.Status.RestoreStatus.ObservedRestoreGeneration,
		)

		var restoreJob batchv1.Job
		restoreJobKey := types.NamespacedName{
			Name:      backupOperator.Name + "-restore",
			Namespace: backupOperator.Namespace,
		}

		err = r.Get(ctx, restoreJobKey, &restoreJob)
		if err != nil && apierrors.IsNotFound(err) {

			// Restore Job not found — create it
			newRestoreJob := r.constructRestoreJob(&backupOperator)
			newRestoreJob.ObjectMeta.Name = backupOperator.Name + "-restore-" + fmt.Sprint(backupOperator.Spec.RestoreGeneration)

			if err := r.Create(ctx, newRestoreJob); err != nil {
				log.Error(err, "Failed to create Restore Job", "BackupOperator", backupOperator.Name)
				return ctrl.Result{}, err
			}
			log.Info("Created Restore Job", "BackupOperator", backupOperator.Name)

			// update status after successful creation
			backupOperator.Status.RestoreStatus.ObservedRestoreGeneration = backupOperator.Spec.RestoreGeneration
			backupOperator.Status.RestoreStatus.RestoreStatus = "Restore job created"
			backupOperator.Status.RestoreStatus.LastRestoreTime = metav1.Now()

			if err := r.Status().Update(ctx, &backupOperator); err != nil {
				log.Error(err, "Failed to update restore status", "BackupOperator", backupOperator.Name)
				return ctrl.Result{}, err
			}

		} else if err != nil {
			log.Error(err, "Failed to get Restore Job", "BackupOperator", backupOperator.Name)
			return ctrl.Result{}, err
		}
		// If job already exists, do nothing — status was already updated when it was created
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackupOperatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1alpha1.BackupOperator{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&batchv1.CronJob{}).
		Owns(&batchv1.Job{}, builder.WithPredicates(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldJob := e.ObjectOld.(*batchv1.Job)
				newJob := e.ObjectNew.(*batchv1.Job)
				return oldJob.Status.Active != newJob.Status.Active ||
					oldJob.Status.Succeeded != newJob.Status.Succeeded ||
					oldJob.Status.Failed != newJob.Status.Failed
			},
			CreateFunc:  func(e event.CreateEvent) bool { return false },
			DeleteFunc:  func(e event.DeleteEvent) bool { return true },
			GenericFunc: func(e event.GenericEvent) bool { return false },
		})).
		Named("backupoperator").
		Complete(r)
}

// helper function
func int32Ptr(i int32) *int32 {
	return &i
}

func (r *BackupOperatorReconciler) constructCronJob(backupOperator *appsv1alpha1.BackupOperator) *batchv1.CronJob {
	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupOperator.Name,
			Namespace: backupOperator.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(backupOperator, appsv1alpha1.GroupVersion.WithKind("BackupOperator")),
			},
		},
		Spec: batchv1.CronJobSpec{
			Schedule:                   backupOperator.Spec.Schedule,
			ConcurrencyPolicy:          batchv1.ForbidConcurrent,
			SuccessfulJobsHistoryLimit: int32Ptr(3),
			FailedJobsHistoryLimit:     int32Ptr(1),
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "backup",
									Image: backupOperator.Spec.BackupImage,
									Command: []string{
										"/bin/sh",
										"-c",
										`
										set -e
										set -o pipefail
										echo "Starting PostgreSQL logical backup..."

										export PGPASSWORD=$POSTGRES_PASSWORD

										echo "Source PVC mounted at /source"
										ls -lh /source

										BACKUP_FILE="/target/postgres-backup-$(date +%Y%m%d-%H%M%S).sql.gz"

										pg_dump \
										-h $POSTGRES_HOST \
										-U $POSTGRES_USER \
										$POSTGRES_DB \
										| gzip > $BACKUP_FILE

										DUMP_EXIT=$?

										if [ $DUMP_EXIT -ne 0 ]; then
										echo "pg_dump failed with exit code $DUMP_EXIT"
										exit 1
										fi

										echo "Backup completed successfully"

										ls -lh $BACKUP_FILE

										echo "Verifying gzip integrity..."

										gzip -t $BACKUP_FILE

										if [ $? -eq 0 ]; then
										echo "Backup integrity verified successfully"
										exit 0
										else
										echo "Backup integrity verification failed"
										exit 1
										fi
 							   `,
									},
									EnvFrom: []corev1.EnvFromSource{
										{
											ConfigMapRef: &corev1.ConfigMapEnvSource{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: "postgres-backup-config",
												},
											},
										},
										{
											SecretRef: &corev1.SecretEnvSource{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: "postgres-backup-secret",
												},
											},
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "source-volume",
											MountPath: "/source",
										},
										{
											Name:      "target-volume",
											MountPath: "/target",
										},
									},
								},
							},
							RestartPolicy: corev1.RestartPolicyOnFailure,
							Volumes: []corev1.Volume{
								{
									Name: "source-volume",
									VolumeSource: corev1.VolumeSource{
										PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: backupOperator.Spec.TargetPVC,
										},
									},
								},
								{
									Name: "target-volume",
									VolumeSource: corev1.VolumeSource{
										PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: backupOperator.Spec.BackupPath,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// helper function to check if the existing CronJob matches the BackupOperator spec
func (r *BackupOperatorReconciler) cronJobMatchesSpec(backupOperator *appsv1alpha1.BackupOperator, cronJob *batchv1.CronJob) bool {
	// Check schedule
	if cronJob.Spec.Schedule != backupOperator.Spec.Schedule {
		return false
	}

	// Check backup image
	if len(cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers) == 0 ||
		cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image != backupOperator.Spec.BackupImage {
		return false
	}

	// Check target PVC in volume mounts
	foundSourceVolume := false
	for _, volume := range cronJob.Spec.JobTemplate.Spec.Template.Spec.Volumes {
		if volume.VolumeSource.PersistentVolumeClaim != nil &&
			volume.VolumeSource.PersistentVolumeClaim.ClaimName == backupOperator.Spec.TargetPVC {
			foundSourceVolume = true
			break
		}
	}
	if !foundSourceVolume {
		return false
	}

	// Check backup path in volume mounts
	foundTargetVolume := false
	for _, volume := range cronJob.Spec.JobTemplate.Spec.Template.Spec.Volumes {
		if volume.VolumeSource.PersistentVolumeClaim != nil &&
			volume.VolumeSource.PersistentVolumeClaim.ClaimName == backupOperator.Spec.BackupPath {
			foundTargetVolume = true
			break
		}
	}
	if !foundTargetVolume {
		return false
	}

	return true
}

type JobStatus struct {
	Phase   string // Active / Complete / Failed / Suspended / Unknown
	Reason  string
	Message string
	Done    bool
}

func CheckJobStatus(job *batchv1.Job) JobStatus {

	// 1. check Failed condition
	if c := getJobCondition(job, batchv1.JobFailed); c != nil {
		if c.Status == corev1.ConditionTrue {
			return JobStatus{
				Phase:   "Failed",
				Reason:  c.Reason,
				Message: c.Message,
				Done:    true,
			}
		}
	}

	// 2. check Complete
	if c := getJobCondition(job, batchv1.JobComplete); c != nil {
		if c.Status == corev1.ConditionTrue {
			return JobStatus{
				Phase:   "Complete",
				Reason:  "JobComplete",
				Message: c.Message,
				Done:    true,
			}
		}
	}

	// 3. check Suspended
	if c := getJobCondition(job, batchv1.JobSuspended); c != nil {
		if c.Status == corev1.ConditionTrue {
			return JobStatus{
				Phase:   "Suspended",
				Reason:  c.Reason,
				Message: c.Message,
				Done:    false,
			}
		}
	}

	// 4. check active pods
	if job.Status.Active > 0 {
		return JobStatus{
			Phase:   "Active",
			Reason:  "JobRunning",
			Message: fmt.Sprintf("%d pod(s) running", job.Status.Active),
			Done:    false,
		}
	}

	// 5. job exists but nothing started yet
	if job.Status.StartTime == nil {
		return JobStatus{
			Phase:   "Pending",
			Reason:  "NotStarted",
			Message: "Job has not started yet",
			Done:    false,
		}
	}

	// 6. fallback
	return JobStatus{
		Phase:   "Unknown",
		Reason:  "UnknownState",
		Message: "Job state cannot be determined",
		Done:    false,
	}
}

// helper
func getJobCondition(job *batchv1.Job, condType batchv1.JobConditionType) *batchv1.JobCondition {
	for i := range job.Status.Conditions {
		if job.Status.Conditions[i].Type == condType {
			return &job.Status.Conditions[i]
		}
	}
	return nil
}

// removeFromSlice removes a string from a slice of strings
func removeFromSlice(slice []string, str string) []string {
	newSlice := []string{}
	for _, s := range slice {
		if s != str {
			newSlice = append(newSlice, s)
		}
	}
	return newSlice
}

// contains checks if a slice of strings contains a specific string
func contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// Creating a new cronjob that will delete the old backup tar files from the backup path.
func (r *BackupOperatorReconciler) constructCleanupCronJob(backupOperator *appsv1alpha1.BackupOperator) *batchv1.CronJob {
	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupOperator.Name + "-cleanup",
			Namespace: backupOperator.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(backupOperator, appsv1alpha1.GroupVersion.WithKind("BackupOperator")),
			},
		},
		Spec: batchv1.CronJobSpec{
			Schedule:                   "0 3 * * 0", // Daily at midnight
			ConcurrencyPolicy:          batchv1.ForbidConcurrent,
			SuccessfulJobsHistoryLimit: int32Ptr(3),
			FailedJobsHistoryLimit:     int32Ptr(1),
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "cleanup",
									Image: "alpine:latest",
									Command: []string{
										"/bin/sh",
										"-c",
										fmt.Sprintf(`
										echo "Starting cleanup of old PostgreSQL backup files in /target..."

										echo "Existing backup files:"
										ls -lh /target

										find /target -type f -name 'postgres-backup-*.sql.gz' -mtime +%d -print -exec rm {} \;

										echo "Remaining backup files after cleanup:"
										ls -lh /target

										echo "Cleanup complete!"
										`, backupOperator.Spec.Retention),
									},
									EnvFrom: []corev1.EnvFromSource{
										{
											ConfigMapRef: &corev1.ConfigMapEnvSource{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: "postgres-backup-config",
												},
											},
										},
										{
											SecretRef: &corev1.SecretEnvSource{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: "postgres-backup-secret",
												},
											},
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "target-volume",
											MountPath: "/target",
										},
									},
								},
							},
							RestartPolicy: corev1.RestartPolicyOnFailure,
							Volumes: []corev1.Volume{
								{
									Name: "target-volume",
									VolumeSource: corev1.VolumeSource{
										PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: backupOperator.Spec.BackupPath,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// Create job for restoring backup files from the backup path to the target PVC. Will be triggered manually through backupOperator field update. This can be used for disaster recovery.
func (r *BackupOperatorReconciler) constructRestoreJob(backupOperator *appsv1alpha1.BackupOperator) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupOperator.Name + "-restore",
			Namespace: backupOperator.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(backupOperator, appsv1alpha1.GroupVersion.WithKind("BackupOperator")),
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "restore",
							Image: backupOperator.Spec.BackupImage,
							Command: []string{
								"/bin/sh",
								"-c",
								fmt.Sprintf(`
								echo "Starting restore of PostgreSQL backup files from /source to /target..."

								export PGPASSWORD=$POSTGRES_PASSWORD
								echo "Existing backup files in source:"
								ls -lh /source

								# Find the newest backup file not older than %d days
								backup_file=$(find /source -name "postgres-backup-*.sql.gz" -type f -mtime -%d | xargs ls -t 2>/dev/null | head -1)

								if [ -z "$backup_file" ]; then
									echo "No backup file found within retention period"
									exit 1
								fi

								echo "Restoring from $backup_file..."
								gunzip -c "$backup_file" | psql -h $POSTGRES_HOST -U $POSTGRES_USER -d $POSTGRES_DB
								if [ $? -eq 0 ]; then
									echo "Restore from $backup_file completed successfully"
								else
									echo "Restore from $backup_file failed"
									exit 1
								fi

								echo "Restore complete!"
							`, backupOperator.Spec.Retention, backupOperator.Spec.Retention),
							},
							EnvFrom: []corev1.EnvFromSource{
								{
									ConfigMapRef: &corev1.ConfigMapEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "postgres-backup-config",
										},
									},
								},
								{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "postgres-backup-secret",
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "source-volume",
									MountPath: "/source",
								},
								{
									Name:      "target-volume",
									MountPath: "/target",
								},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Volumes: []corev1.Volume{
						{
							Name: "source-volume",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: backupOperator.Spec.BackupPath,
								},
							},
						},
						{
							Name: "target-volume",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: backupOperator.Spec.TargetPVC,
								},
							},
						},
					},
				},
			},
		},
	}
}
