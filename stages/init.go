package stages

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/kairos-io/kairos/provider-kubeadm/domain"

	"github.com/kairos-io/kairos-sdk/clusterplugin"
	"github.com/kairos-io/kairos/provider-kubeadm/utils"
	yip "github.com/mudler/yip/pkg/schema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/printers"
	bootstraputil "k8s.io/cluster-bootstrap/token/util"
	kubeletv1beta1 "k8s.io/kubelet/config/v1beta1"
	bootstraptokenv1 "k8s.io/kubernetes/cmd/kubeadm/app/apis/bootstraptoken/v1"
	kubeadmapiv3 "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta3"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	_ = kubeadmapiv3.AddToScheme(scheme)
	_ = kubeletv1beta1.AddToScheme(scheme)
}

const (
	configurationPath = "opt/kubeadm"
)

func GetInitYipStages(cluster clusterplugin.Cluster, initCfg kubeadmapiv3.InitConfiguration, clusterCfg kubeadmapiv3.ClusterConfiguration, kubeletCfg kubeletv1beta1.KubeletConfiguration) []yip.Stage {
	utils.MutateClusterConfigDefaults(cluster, &clusterCfg)
	utils.MutateKubeletDefaults(&clusterCfg, &kubeletCfg)
	clusterRootPath := utils.GetClusterRootPath(cluster)
	return []yip.Stage{
		getKubeadmInitConfigStage(getInitNodeConfiguration(cluster, initCfg, clusterCfg, kubeletCfg), clusterRootPath),
		getKubeadmInitStage(cluster, clusterCfg),
		getKubeadmPostInitStage(cluster),
		getKubeadmInitUpgradeStage(cluster, clusterCfg),
		getKubeadmInitCreateClusterConfigStage(clusterCfg, initCfg, clusterRootPath),
		getKubeadmInitCreateKubeletConfigStage(kubeletCfg, clusterRootPath),
		getKubeadmInitReconfigureStage(cluster, clusterCfg, initCfg),
	}
}

func getKubeadmInitConfigStage(kubeadmCfg, rootPath string) yip.Stage {
	return yip.Stage{
		Name: "Generate Kubeadm Init Config File",
		Files: []yip.File{
			{
				Path:        filepath.Join(rootPath, configurationPath, "kubeadm.yaml"),
				Permissions: 0640,
				Content:     kubeadmCfg,
			},
		},
	}
}

func getKubeadmInitStage(cluster clusterplugin.Cluster, clusterCfg kubeadmapiv3.ClusterConfiguration) yip.Stage {
	clusterRootPath := utils.GetClusterRootPath(cluster)

	initStage := yip.Stage{
		Name: "Run Kubeadm Init",
		If:   fmt.Sprintf("[ ! -f %s ]", filepath.Join(clusterRootPath, "opt/kubeadm.init")),
	}

	if utils.IsProxyConfigured(cluster.Env) {
		proxy := cluster.Env
		initStage.Commands = []string{
			fmt.Sprintf("bash %s %s %t %s %s %s", filepath.Join(clusterRootPath, helperScriptPath, "kube-init.sh"), clusterRootPath, true, proxy["HTTP_PROXY"], proxy["HTTPS_PROXY"], utils.GetNoProxyConfig(clusterCfg, cluster.Env)),
			fmt.Sprintf("touch %s", filepath.Join(clusterRootPath, "opt/kubeadm.init")),
		}
	} else {
		initStage.Commands = []string{
			fmt.Sprintf("bash %s %s", filepath.Join(clusterRootPath, helperScriptPath, "kube-init.sh"), clusterRootPath),
			fmt.Sprintf("touch %s", filepath.Join(clusterRootPath, "opt/kubeadm.init")),
		}
	}
	return initStage
}

func getKubeadmPostInitStage(cluster clusterplugin.Cluster) yip.Stage {
	clusterRootPath := utils.GetClusterRootPath(cluster)

	return yip.Stage{
		Name: "Run Post Kubeadm Init",
		If:   fmt.Sprintf("[ ! -f %s ]", filepath.Join(clusterRootPath, "opt/post-kubeadm.init")),
		Commands: []string{
			fmt.Sprintf("bash %s %s", filepath.Join(clusterRootPath, helperScriptPath, "kube-post-init.sh"), clusterRootPath),
			fmt.Sprintf("touch %s", filepath.Join(clusterRootPath, "opt/post-kubeadm.init")),
		},
	}
}

func getKubeadmInitUpgradeStage(cluster clusterplugin.Cluster, clusterCfg kubeadmapiv3.ClusterConfiguration) yip.Stage {
	upgradeStage := yip.Stage{
		Name: "Run Kubeadm Init Upgrade",
	}
	clusterRootPath := utils.GetClusterRootPath(cluster)

	if utils.IsProxyConfigured(cluster.Env) {
		proxy := cluster.Env
		upgradeStage.Commands = []string{
			fmt.Sprintf("bash %s %s %s %t %s %s %s", filepath.Join(clusterRootPath, helperScriptPath, "kube-upgrade.sh"), cluster.Role, clusterRootPath, true, proxy["HTTP_PROXY"], proxy["HTTPS_PROXY"], utils.GetNoProxyConfig(clusterCfg, cluster.Env)),
		}
	} else {
		upgradeStage.Commands = []string{
			fmt.Sprintf("bash %s %s %s", filepath.Join(clusterRootPath, helperScriptPath, "kube-upgrade.sh"), cluster.Role, clusterRootPath),
		}
	}
	return upgradeStage
}

func getKubeadmInitCreateClusterConfigStage(clusterCfg kubeadmapiv3.ClusterConfiguration, initCfg kubeadmapiv3.InitConfiguration, rootPath string) yip.Stage {
	return yip.Stage{
		Name: "Generate Cluster Config File",
		Files: []yip.File{
			{
				Path:        filepath.Join(rootPath, configurationPath, "cluster-config.yaml"),
				Permissions: 0640,
				Content:     getUpdatedInitClusterConfig(clusterCfg, initCfg),
			},
		},
	}
}

func getKubeadmInitCreateKubeletConfigStage(kubeletCfg kubeletv1beta1.KubeletConfiguration, rootPath string) yip.Stage {
	return yip.Stage{
		Name: "Generate Kubelet Config File",
		Files: []yip.File{
			{
				Path:        filepath.Join(rootPath, configurationPath, "kubelet-config.yaml"),
				Permissions: 0640,
				Content:     getUpdatedKubeletConfig(kubeletCfg),
			},
		},
	}
}

func getKubeadmInitReconfigureStage(cluster clusterplugin.Cluster, clusterCfg kubeadmapiv3.ClusterConfiguration, initCfg kubeadmapiv3.InitConfiguration) yip.Stage {
	reconfigureStage := yip.Stage{
		Name: "Run Kubeadm Reconfiguration",
	}

	clusterRootPath := utils.GetClusterRootPath(cluster)
	kubeletArgs := utils.RegenerateKubeletKubeadmArgsFile(&clusterCfg, &initCfg.NodeRegistration, string(cluster.Role))
	sansRevision := utils.GetCertSansRevision(clusterCfg.APIServer.CertSANs)

	if utils.IsProxyConfigured(cluster.Env) {
		proxy := cluster.Env
		reconfigureStage.Commands = []string{
			fmt.Sprintf("bash %s %s %s %s %s %s %s %s", filepath.Join(clusterRootPath, helperScriptPath, "kube-reconfigure.sh"), cluster.Role, sansRevision, kubeletArgs, clusterRootPath, proxy["HTTP_PROXY"], proxy["HTTPS_PROXY"], utils.GetNoProxyConfig(clusterCfg, cluster.Env)),
		}
	} else {
		reconfigureStage.Commands = []string{
			fmt.Sprintf("bash %s %s %s %s %s", filepath.Join(clusterRootPath, helperScriptPath, "kube-reconfigure.sh"), cluster.Role, sansRevision, kubeletArgs, clusterRootPath),
		}
	}
	return reconfigureStage
}

func getInitNodeConfiguration(cluster clusterplugin.Cluster, initCfg kubeadmapiv3.InitConfiguration, clusterCfg kubeadmapiv3.ClusterConfiguration, kubeletCfg kubeletv1beta1.KubeletConfiguration) string {
	certificateKey := utils.GetCertificateKey(cluster.ClusterToken)

	substrs := bootstraputil.BootstrapTokenRegexp.FindStringSubmatch(cluster.ClusterToken)

	initCfg.BootstrapTokens = []bootstraptokenv1.BootstrapToken{
		{
			Token: &bootstraptokenv1.BootstrapTokenString{
				ID:     substrs[1],
				Secret: substrs[2],
			},
			TTL: &metav1.Duration{
				Duration: 0,
			},
		},
	}
	initCfg.CertificateKey = certificateKey

	var apiEndpoint kubeadmapiv3.APIEndpoint

	if initCfg.LocalAPIEndpoint.AdvertiseAddress == "" {
		apiEndpoint.AdvertiseAddress = domain.DefaultAPIAdvertiseAddress
	} else {
		apiEndpoint.AdvertiseAddress = initCfg.LocalAPIEndpoint.AdvertiseAddress
	}

	if initCfg.LocalAPIEndpoint.BindPort != 0 {
		apiEndpoint.BindPort = initCfg.LocalAPIEndpoint.BindPort
	}

	initCfg.LocalAPIEndpoint = apiEndpoint

	initPrintr := printers.NewTypeSetter(scheme).ToPrinter(&printers.YAMLPrinter{})

	out := bytes.NewBuffer([]byte{})

	_ = initPrintr.PrintObj(&clusterCfg, out)
	_ = initPrintr.PrintObj(&initCfg, out)
	_ = initPrintr.PrintObj(&kubeletCfg, out)

	return out.String()
}

func getUpdatedInitClusterConfig(clusterCfg kubeadmapiv3.ClusterConfiguration, initCfg kubeadmapiv3.InitConfiguration) string {
	initPrintr := printers.NewTypeSetter(scheme).ToPrinter(&printers.YAMLPrinter{})

	out := bytes.NewBuffer([]byte{})
	_ = initPrintr.PrintObj(&clusterCfg, out)
	_ = initPrintr.PrintObj(&initCfg, out)

	return out.String()
}

func getUpdatedKubeletConfig(kubeletCfg kubeletv1beta1.KubeletConfiguration) string {
	initPrintr := printers.NewTypeSetter(scheme).ToPrinter(&printers.YAMLPrinter{})

	out := bytes.NewBuffer([]byte{})
	_ = initPrintr.PrintObj(&kubeletCfg, out)

	return out.String()
}
