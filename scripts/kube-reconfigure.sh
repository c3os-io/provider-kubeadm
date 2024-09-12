#!/bin/bash

set -x
trap 'echo -n $(date)' DEBUG

exec   > >(tee -ia /var/log/kube-reconfigure.log)
exec  2> >(tee -ia /var/log/kube-reconfigure.log >& 2)

info() {
    echo "[INFO] " "$@"
}

node_role=$1
certs_sans_revision=$2
kubelet_envs=$3
root_path=$4
proxy_http=$5
proxy_https=$6
proxy_no=$7

export PATH="$PATH:$root_path/usr/bin"

certs_sans_revision_path="$root_path/opt/kubeadm/.kubeadm_certs_sans_revision"

if [ -n "$proxy_no" ]; then
  export NO_PROXY=$proxy_no
  export no_proxy=$proxy_no
fi

if [ -n "$proxy_http" ]; then
  export HTTP_PROXY=$proxy_http
  export http_proxy=$proxy_http
fi

if [ -n "$proxy_https" ]; then
  export https_proxy=$proxy_https
  export HTTPS_PROXY=$proxy_https
fi

export KUBECONFIG=/etc/kubernetes/admin.conf

regenerate_kube_components_manifests() {
  sudo -E bash -c "kubeadm init phase control-plane apiserver --config $root_path/opt/kubeadm/cluster-config.yaml"
  sudo -E bash -c "kubeadm init phase control-plane controller-manager --config $root_path/opt/kubeadm/cluster-config.yaml"
  sudo -E bash -c "kubeadm init phase control-plane scheduler --config $root_path/opt/kubeadm/cluster-config.yaml"

  kubeadm init phase upload-config kubeadm --config "$root_path"/opt/kubeadm/cluster-config.yaml

  info "regenerated kube components manifest"
}

regenerate_apiserver_certs_sans() {
  if [ ! -f "$certs_sans_revision_path" ]; then
    echo "$certs_sans_revision" > "$certs_sans_revision_path"
    return
  fi

  current_revision=$(cat "$certs_sans_revision_path")

  if [ "$certs_sans_revision" = "$current_revision" ]; then
    info "no change in certs sans revision"
    return
  fi

  rm /etc/kubernetes/pki/apiserver.{crt,key}
  info "regenerated removed existing apiserver certs"

  kubeadm init phase certs apiserver --config "$root_path"/opt/kubeadm/cluster-config.yaml
  info "regenerated apiserver certs"

  crictl pods 2>/dev/null | grep kube-apiserver | cut -d' ' -f1 | xargs -I %s sh -c '{ crictl stopp %s; crictl rmp %s; }' 2>/dev/null
  info "deleted existing apiserver pod"

  kubeadm init phase upload-config kubeadm --config "$root_path"/opt/kubeadm/cluster-config.yaml

  restart_kubelet
}

regenerate_kubelet_envs() {
  echo "$kubelet_envs" > /var/lib/kubelet/kubeadm-flags.env
  systemctl restart kubelet
}

regenerate_kubelet_config() {
  kubeadm upgrade node phase kubelet-config
}

upload_kubelet_config() {
  kubeadm init phase upload-config kubelet --config "$root_path"/opt/kubeadm/kubelet-config.yaml
}

restart_kubelet() {
  systemctl restart kubelet
}

regenerate_etcd_manifests() {
  until kubectl --kubeconfig=/etc/kubernetes/admin.conf get cs > /dev/null
  do
    info "generating etcd manifests, cluster not accessible, retrying after 60 sec"
    sleep 60
    continue
  done
  kubeadm init phase etcd local --config "$root_path"/opt/kubeadm/cluster-config.yaml
  info "regenerated etcd manifest"
  sleep 60
}

if [ "$node_role" != "worker" ];
then
  regenerate_kube_components_manifests
  regenerate_apiserver_certs_sans
  regenerate_etcd_manifests
  upload_kubelet_config
fi
regenerate_kubelet_config
regenerate_kubelet_envs
restart_kubelet