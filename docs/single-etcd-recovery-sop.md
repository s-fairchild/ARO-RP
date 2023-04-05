# Single Master Etcd Recovery Standard Operating Procedure

### Overview

Running the Etcd Recovery PUCM step, at a high level, follows this workflow:

1. A patch is sent to the RP with maintenance task set to `EtcdRecovery`
2. The condition is verified by comparing all etcd Pod Spec Environments to that node's actual IP address
   1. If the condition isn't found, the task exits with an error here
3. Etcd data is backed up
   1. A pod is created that chroot's into the node's filesystem
   2. Etcd's data directories and manifest are moved to `/tmp` and `/var/lib/etcd-backup` respectively
4. The failing etcd member is removed from its healthy peers
   1. A pod is created that ssh's into each etcd pod and runs `etcdctl member remove "$id"`
5. Etcd is patched with `{"useUnsupportedUnsafeNonHANonProductionUnstableEtcd": true}`
   1. This allows the pod's to reconcile by running in a non HA
6. Delete secrets (name's will reflect actual IP addresses)
   1. etcd-peer-ip-10-0-131-183.ec2.internal
   2. etcd-serving-ip-10-0-131-183.ec2.internal
   3. etcd-serving-metrics-ip-10-0-131-183.ec2.internal
7. Etcd is patched to disable unsupported overrides
   1. PUCM task ends
8. The etcd quorum will begin to reconcile itself
   1. This may take some time as each etcd pod restarts

### Approval

The approval required to perform these actions are

1. 

### How to use?

1. How to use locally with `curl`
   1. ```bash
       curl -X PATCH -k "https://localhost:8443/subscriptions/$AZURE_SUBSCRIPTION_ID/resourceGroups/$RESOURCEGROUP/providers/Microsoft.RedHatOpenShift/openShiftClusters/$CLUSTER?api-version=admin" --header "Content-Type: application/json" -d '{"properties": {"maintenanceTask": "EtcdRecovery"}}'
       ```
2. How to use via Geneva Actions 
   1. | Action |                       Contents                        |
      |:-----------------------------------------------------:| :---: |
      | Patch Cluster | `{"properties": {"maintenanceTask": "EtcdRecovery"}}` |

### How to Identify?

1. A crash looping etcd pod 
   1. Review the etcd pods in `openshift-etcd`
   2. Examine the Pod Spec environment for discrepancies between `ALL_ETCD_ENDPOINTS`, `NODE_${CLUSTERNAME}_master_0_ETCD_URL_HOST` and `NODE_${CLUSTERNAME}_master_0_IP` within the same Pod Spec
   3. Compare the crashlooping Pod Spec's `ALL_ETCD_ENDPOINTS` it's member pod's `ALL_ETCD_ENDPOINTS` looking for any IP address discrepancies
2. ```bash
   W1215 14:41:58.915994       1 clientconn.go:1208] grpc: addrConn.createTransport failed to connect to {https://a.b.c.d:2379  <nil> 0 <nil>}. Err :connection error: desc = "transport: Error while dialing dial tcp a.v.c.d:2379: connect: no route to host". Reconnecting...
   ```
   This log message indicates that the master nodes IP address has changed, causing connection failures. The log entry comes from etcd-operator from openshift-etcd-operator namespace

### References

- [Replacing an Unhealthy Etcd Member - OCP 4.10](https://docs.openshift.com/container-platform/4.10/backup_and_restore/control_plane_backup_and_restore/replacing-unhealthy-etcd-member.html)
