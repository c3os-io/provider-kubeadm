#!/bin/bash

exec   > >(tee -ia /var/log/kube-init.log)
exec  2> >(tee -ia /var/log/kube-init.log >& 2)
exec 19>> /var/log/kube-init.log

export BASH_XTRACEFD="19"
set -ex

KUBE_VIP_LOC="/etc/kubernetes/manifests/kube-vip.yaml"

until kubeadm init --config /opt/kubeadm/kubeadm.yaml --upload-certs --ignore-preflight-errors=DirAvailable--etc-kubernetes-manifests -v=5 > /dev/null
do
  backup_kube_vip_manifest_if_present
  echo "failed to apply kubeadm init, applying reset";
  do_kubeadm_reset
  echo "retrying in 10s"
  sleep 10;
  restore_kube_vip_manifest_after_reset
done;

do_kubeadm_reset() {
  kubeadm reset -f
  iptables -F && iptables -t nat -F && iptables -t mangle -F && iptables -X && rm -rf /etc/kubernetes/etcd /etc/kubernetes/manifests /etc/kubernetes/pki
  rm -rf /etc/cni/net.d
}

backup_kube_vip_manifest_if_present() {
  if [ -f "$KUBE_VIP_LOC" ]; then
    cp $KUBE_VIP_LOC /opt/kubeadm/kube-vip.yaml
  fi
}

restore_kube_vip_manifest_after_reset() {
  if [ -f "/opt/kubeadm/kube-vip.yaml" ]; then
      mkdir -p /etc/kubernetes/manifests
      cp /opt/kubeadm/kube-vip.yaml $KUBE_VIP_LOC
  fi
}