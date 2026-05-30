# PostgreSQL Backup & Restore Operator

A Kubernetes Operator built using Kubebuilder and Go that automates PostgreSQL backup, retention management, and disaster recovery workflows inside Kubernetes.

## Overview

Managing database backups is a critical operational responsibility in Kubernetes environments. While Kubernetes provides CronJobs and Jobs, configuring, maintaining, and recovering PostgreSQL backups manually can become repetitive and error-prone.

The PostgreSQL Backup & Restore Operator automates the entire lifecycle of database protection:

* Scheduled PostgreSQL backups
* Compressed backup storage
* Backup retention management
* Automatic cleanup of old backups
* On-demand disaster recovery through restore jobs
* Status tracking through Kubernetes Custom Resources

The operator follows Kubernetes-native patterns using Custom Resources, CronJobs, Jobs, ConfigMaps, Secrets, and Persistent Volume Claims.

---

## Features

### Automated Backups

* Schedule PostgreSQL backups using cron expressions
* Uses `pg_dump` for logical database backups
* Stores compressed backups as `.sql.gz`

### Backup Storage

* Stores backups on a dedicated Persistent Volume Claim
* Supports long-term backup retention

### Retention Management

* Automatically removes old backup files
* Configurable retention period

### Disaster Recovery

* Trigger database restoration using generation-based requests
* Restore from the most recent available backup
* Uses PostgreSQL native restore workflow

### Status Tracking

Tracks:

* Last backup execution
* Backup success/failure status
* Active backup jobs
* Restore requests
* Restore execution history

---

## Architecture

```text
                     BackupOperator CR
                              |
                              |
                    +---------+---------+
                    |                   |
                    v                   v
            Backup CronJob      Cleanup CronJob
                    |                   |
                    v                   |
             PostgreSQL Backup          |
             (pg_dump + gzip)           |
                    |                   |
                    +---------+---------+
                              |
                              v
                       Backup Storage PVC
                              |
                              v
                       Restore Job
                              |
                              v
                     PostgreSQL Database
```

---

## Custom Resource Example

```yaml
apiVersion: apps.dev.com/v1alpha1
kind: BackupOperator

metadata:
  name: postgres-backup

spec:
  restoreGeneration: 0

  targetPVC: postgres-pvc

  schedule: "0 2 * * *"

  backupImage: postgres:16

  backupPath: backup-storage-pvc

  retention: 2
```

---

## CRD Fields

| Field             | Description                                 |
| ----------------- | ------------------------------------------- |
| restoreGeneration | Generation-based restore trigger            |
| targetPVC         | PostgreSQL data PVC                         |
| schedule          | Backup schedule (Cron format)               |
| backupImage       | Container image used for backup and restore |
| backupPath        | PVC used to store backup files              |
| retention         | Retention period for backups                |

---

## Backup Workflow

1. Backup CronJob runs on the configured schedule.
2. PostgreSQL credentials are loaded from ConfigMap and Secret.
3. `pg_dump` creates a logical database backup.
4. Backup is compressed using gzip.
5. Backup file is stored on the backup PVC.
6. Operator updates backup status.

### Example Backup File

```text
postgres-backup-20260530-021500.sql.gz
```

---

## Cleanup Workflow

1. Cleanup CronJob executes periodically.
2. Backup files older than the configured retention period are identified.
3. Expired backups are removed.
4. Backup storage remains clean and manageable.

---

## Restore Workflow

The operator uses a generation-based restore mechanism.

Initial state:

```yaml
spec:
  restoreGeneration: 0
```

To request a restore:

```yaml
spec:
  restoreGeneration: 1
```

To request another restore later:

```yaml
spec:
  restoreGeneration: 2
```

The operator compares:

```text
spec.restoreGeneration
```

against:

```text
status.observedRestoreGeneration
```

and creates a restore Job only when a new generation is detected.

### Restore Process

1. User increments `restoreGeneration`
2. Operator creates Restore Job
3. Latest backup is identified
4. Backup is decompressed
5. SQL is restored using `psql`
6. Restore status is updated

---

## Status Example

```yaml
status:
  lastBackupTime:
  lastBackupStatus:
  activeBackupJobs:

  restoreStatus:
    observedRestoreGeneration:
    lastRestoreTime:
    restoreStatus:
```

---

## Disaster Recovery Test

The operator has been validated using the following workflow:

1. Create PostgreSQL table and data
2. Generate backup
3. Delete table
4. Trigger restore by incrementing `restoreGeneration`
5. Verify table and data are restored successfully

This confirms end-to-end backup and recovery functionality.

---

## Technologies Used

* Go
* Kubernetes
* Kubebuilder
* controller-runtime
* PostgreSQL
* CronJobs
* Jobs
* ConfigMaps
* Secrets
* Persistent Volume Claims

---

## Learning Outcomes

This project demonstrates:

* Kubernetes Operator Development
* Custom Resource Design
* Reconciliation Loops
* Status Management
* CronJob Orchestration
* Job Orchestration
* PostgreSQL Backup & Recovery
* Disaster Recovery Workflows
* Watches and Predicates
* Owner References
* Kubernetes Storage Management
* Generation-Based Operations

---

## Future Enhancements

* Backup selection during restore
* S3 / Object Storage support
* Backup encryption
* Backup checksum validation
* Prometheus metrics
* Webhook validation
* Multi-database support
* Scheduled restore testing

---

## License

Licensed under the Apache License 2.0.
