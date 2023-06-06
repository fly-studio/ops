package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"
	"net"
	nos "os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"stkey/internal/content"
	"stkey/pkg/logger"
	"stkey/pkg/os"
	"stkey/pkg/script"
	"stkey/utils"
	"strings"
	"time"
)

func buildInitCmd() *cobra.Command {
	var osInfo *os.Data

	initCmd := &cobra.Command{
		Use:   "init [Commands...] -x <Command> -x <Command>",
		Short: "初始化OS,支持CentOS 6/7/8,Ubuntu 18/20/22",
		Long: `用法: init [Commands...]:
Commands:
    kernel          更新内核参数
    system          优化系统设置
    time            安装chrony、设置时区Asia/Shanghai
    pkg             安装YUM或APT源仓库及依赖工具
    docker          安装docker
	tools		    安装常用工具
    all             执行所有指令
`,
		Args: cobra.MinimumNArgs(1),
		PreRun: func(cmd *cobra.Command, args []string) {
			osInfo = checkGOOS()
			checkUserPermission()
			disableUbuntuAutoUpgrade(osInfo)
		},
		Run: func(cmd *cobra.Command, args []string) {

			allOptions := []string{"kernel", "system", "time", "pkg", "docker", "tools"}
			except, _ := cmd.Flags().GetStringSlice("except")

			// 如果参数包含all,则执行所有指令
			if slices.Contains(args, "all") {
				args = allOptions
			} else {
				// 检查参数是否合法
				for _, arg := range args {
					if !slices.Contains(allOptions, arg) {
						logger.Sugar.Fatalf("不支持的参数: %s", arg)
						return
					}
				}

				// 按照allOptions的顺序排序
				sort.Slice(args, func(i, j int) bool {
					return slices.Index(allOptions, args[i]) < slices.Index(allOptions, args[j])
				})
			}

			for _, option := range args {
				// 排除不需要执行的指令
				if slices.Contains(except, option) {
					continue
				}

				switch option {
				case "kernel":
					updateKernel(osInfo)
				case "system":
					optimizeSystem(osInfo)
				case "time":
					syncTime(osInfo)
				case "pkg":
					updatePkg(osInfo)
				case "docker":
					installDocker(osInfo)
				case "tools":
					downloadTools()
				}
			}

		},
		PostRun: func(cmd *cobra.Command, args []string) {
			enableUbuntuAutoUpgrade(osInfo)
		},
	}

	initCmd.Flags().StringSliceP("except", "x", []string{}, "排除这些指令，比如排除這2個：-x docker -x tools")

	return initCmd
}

func checkGOOS() *os.Data {
	var sysType string = runtime.GOOS
	var o *os.Data
	if sysType == "windows" {
		o = &os.Data{}
	} else if sysType == "linux" {
		o = os.Parse(os.EtcOsRelease, os.IssueOsRelease)
	}
	return o
}

// 优化内核设置
func updateKernel(osInfo *os.Data) {
	logger.Sugar.Infoln("开始检查并更新内核参数")
	var modules []string
	if osInfo.IsCentOS6() {
		modules = []string{"ip_vs", "ip_vs_rr", "ip_vs_wrr", "ip_vs_sh", "nf_conntrack"}
	} else {
		modules = []string{"br_netfilter", "ip_vs", "ip_vs_rr", "ip_vs_wrr", "ip_vs_sh", "nf_conntrack"}
	}
	p, _ := script.File(osInfo.FileMap["modulePath"]).Match(modules[0]).String()
	if len(p) == 0 {
		for _, m := range modules {
			_, _ = script.Echo(m + "\n").AppendFile(osInfo.FileMap["modulePath"])
		}
	}
	p, _ = script.File(osInfo.FileMap["rcLocalPath"]).Match(modules[0]).String()
	if len(p) == 0 && osInfo.IsCentOS() {
		for _, m := range modules {
			_, _ = script.Echo("modprobe " + m + "\n").AppendFile(osInfo.FileMap["rcLocalPath"])
			_, _ = script.Exec("sudo chmod +x " + osInfo.FileMap["rcLocalPath"]).Stdout()
		}
	}
	for _, m := range modules {
		_, _ = script.Exec("sudo modprobe " + m).Stdout()
	}
	//centos7新增fs.may_detach_mounts
	if osInfo.IsCentOS7() {
		_ = utils.AppendFileIf(osInfo.FileMap["sysctlPath"], "fs.may_detach_mounts", "fs.may_detach_mounts = 1\n")
	}
	if osInfo.IsCentOS6() {
		_ = utils.AppendFileIf(osInfo.FileMap["sysctlPath"], "start check kernel", content.CentOs6SysctlText)
	} else {
		_ = utils.AppendFileIf(osInfo.FileMap["sysctlPath"], "start check kernel", content.SysctlText)
	}
	s := script.Exec("sudo sysctl -p")
	s.Wait()
	if s.ExitStatus() != 0 {
		logger.Sugar.Fatal("sysctl -p 更新sysctl失败,请检查")
	} else {
		logger.Sugar.Infoln("sysctl -p:")
		_, _ = script.Exec("sudo sysctl -p").Stdout()
		logger.Sugar.Infoln("更新内核参数成功")
	}
}

// 关闭swap
func disableSwap() {
	logger.Sugar.Infoln("关闭swap")
	_, _ = script.Exec("sudo swapoff -a").Stdout()
}

// 检查设置limit
func updateLimit(osInfo *os.Data) {
	logger.Sugar.Infoln("检查系统Limit设置")
	if osInfo.IsUbuntu() {
		_ = utils.AppendFileIf("/etc/security/limits.conf", "* soft nofile 102400", content.UbuntuLimitConf)
	} else {
		_ = utils.AppendFileIf("/etc/security/limits.conf", "* soft nofile 102400", content.DefaultLimitConf)
	}

	if !osInfo.IsCentOS6() {
		_ = utils.Replace("/etc/systemd/system.conf", "#DefaultLimitNOFILE=", "DefaultLimitNOFILE=102400")
		_ = utils.Replace("/etc/systemd/system.conf", "#DefaultLimitNPROC=", "DefaultLimitNPROC=102400")
	}
	_ = exec.Command("/bin/bash", "-c", "ulimit -n 65535").Run()
}

// 检查bashrc
func updateBashrc() {
	logger.Sugar.Infoln("检查用户Bashrc设置")
	_, _ = script.Echo(content.BashrcConf).WriteFile("/root/.bashrc")
	if utils.PathExists("/etc/bashrc") {
		_, _ = script.Echo(content.TerminalConf).AppendFile("/etc/bashrc")
		_ = exec.Command("/bin/bash", "-c", "source /etc/bashrc").Run()
	}

	if utils.PathExists("/etc/bash.bashrc") {
		_, _ = script.Echo(content.TerminalConf).AppendFile("/etc/bash.bashrc")
		_ = exec.Command("/bin/bash", "-c", "source /etc/bash.bashrc").Run()
	}
}

// CentOS关闭selinux,firewalld
func disableDefault(osInfo *os.Data) {
	logger.Sugar.Infoln("检查并关闭SELinux,FireWalld(如果存在)")
	if osInfo.IsCentOS() {
		_, _ = script.Exec("sudo setenforce 0").Stdout()
		_ = utils.Replace("/etc/selinux/config", "SELINUX=enforcing", "SELINUX=disabled")
	}

	if osInfo.IsCentOS() && !osInfo.IsCentOS6() {
		if utils.TryCommand("firewall-cmd") {
			_, _ = script.Exec("sudo systemctl stop firewalld").Stdout()
			_, _ = script.Exec("sudo systemctl disable firewalld").Stdout()
		}
	}
}

// 设置history命令记录
func updateHistory() {
	logger.Sugar.Infoln("设置history命令记录,history记录目录:/var/log/.hist")
	err := utils.MustMakeDir("/var/log/.hist")
	if err != nil {
		logger.Sugar.Fatal("updateHistory()", err)
	}

	time.Sleep(time.Duration(1) * time.Second)
	if utils.PathExists("/etc/bashrc") {
		_, _ = script.Echo(content.HistoryLog).WriteFile("/etc/bashrc")
		_ = exec.Command("/bin/bash", "-c", "source /etc/bashrc").Run()
	}

	if utils.PathExists("/etc/bash.bashrc") {
		_, _ = script.Echo(content.HistoryLog).WriteFile("/etc/bash.bashrc")
		_ = exec.Command("/bin/bash", "-c", "source /etc/bash.bashrc").Run()
	}

	logger.Sugar.Infoln("添加history logrotate")
	_, _ = script.Echo(content.LogrotateHistory).WriteFile("/etc/logrotate.d/command")
	_, _ = script.Exec("sudo chmod -R 777 " + "/var/log/.hist").Stdout()
}

func optimizeSystem(osInfo *os.Data) {
	disableSwap()
	updateLimit(osInfo)
	updateBashrc()
	disableDefault(osInfo)
	updateHistory()
}

var spaceRegexp = regexp.MustCompile(`\s+`)
var lineRegexp = regexp.MustCompile(`\n+`)

// 下载更新s3存储上的相关工具，文件列表：https://s3.load.cool:8000/linux-software.txt
func downloadTools() {
	logger.Sugar.Infoln("检查安装os相关command")
	_ = utils.MustMakeDir("/usr/libexec/docker/cli-plugins/")
	time.Sleep(time.Duration(1) * time.Second)

	_content := utils.HttpGet("https://s3.load.cool:8000/linux-software.txt")
	if _content == "" {
		logger.Sugar.Errorln("获取文件列表失败")
		return
	}
	for _, line := range lineRegexp.Split(strings.TrimSpace(_content), -1) {
		segments := spaceRegexp.Split(strings.TrimSpace(line), -1)
		if len(segments) < 3 {
			logger.Sugar.Infoln("格式错误，跳过：" + line)
			continue
		}

		_url := segments[0]
		_md5 := segments[1]
		_paths := segments[2:]

		for _, _path := range _paths {
			// 如果文件不存在，下载
			if !utils.PathExists(_path) {
				_ = utils.MustMakeDir(filepath.Dir(_path))
				logger.Sugar.Infoln("文件不存在开始下载：" + _url + " --->" + _path)
				if err := utils.DownloadFile(_path, _url); err != nil {
					logger.Sugar.Errorln(err)
				}
			} else {
				// 如果文件存在，但是md5不匹配，下载
				currentMd5 := utils.MD5File(_path)
				if strings.EqualFold(currentMd5, _md5) {
					logger.Sugar.Infoln("MD5不匹配，开始下载：" + _url + " --->" + _path)
					if err := utils.DownloadFile(_path, _url); err != nil {
						logger.Sugar.Errorln(err)
					}
				} else {
					logger.Sugar.Infoln("文件已存在並且MD5相符，跳过：" + _path)
				}
			}
			script.Exec("sudo chmod +x " + _path).Stdout()
		}
	}
}

// 安装设置Chrony时间同步
func syncTime(osInfo *os.Data) {
	logger.Sugar.Infoln("安装配置chrony时间同步")
	if osInfo.IsLikeFedora() {
		ntpConf := "/etc/chrony.conf"
		ntpConfPath := content.FedoraChronyConf
		if !utils.TryCommand("chronyd") {
			logger.Sugar.Infoln("检测到chrony服务不存在,开始安装chrony")
			_, err := script.Exec("sudo yum install chrony -y").Stdout()
			if err != nil {
				logger.Sugar.Fatal(err)
			}
		} else {
			logger.Sugar.Infoln("检测到chrony服务已存在，开始设置chrony服务")
		}
		if utils.PathExists(ntpConf) {
			script.Echo(ntpConfPath).WriteFile(ntpConf)
		} else {
			logger.Sugar.Fatalf("配置文件%s不存在，请检查chrony服务", ntpConf)
		}

		if osInfo.IsCentOS6() {
			_, err := script.Exec("sudo /etc/init.d/chronyd start").Stdout()
			if err != nil {
				logger.Sugar.Fatal(err)
			}
			_, err = script.Exec("sudo chkconfig chronyd on").Stdout()
			if err != nil {
				logger.Sugar.Fatal(err)
			}
			_, err = script.Exec("sudo ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime").Stdout()
			if err != nil {
				logger.Sugar.Infoln("时区设置失败")
			} else {
				logger.Sugar.Infof("更新chrony配置文件:%s", ntpConf)
				script.File(ntpConf).Stdout()
				logger.Sugar.Infoln("chrony同步状态:")
				script.Exec("chronyc sources -v").Stdout()
				logger.Sugar.Infoln("chrony设置成功")
			}
		} else {
			if osInfo.IsLikeFedora() && !osInfo.IsCentOS6() {
				_, _ = script.Exec("sudo systemctl restart chronyd").Stdout()
				_, err := script.Exec("sudo systemctl enable chronyd --now").Stdout()
				if err != nil {
					logger.Sugar.Fatal(err)
				}
			} else if osInfo.IsLikeDebian() {
				_, _ = script.Exec("sudo systemctl restart chrony").Stdout()
				_, err := script.Exec("sudo systemctl enable chrony --now").Stdout()
				if err != nil {
					logger.Sugar.Fatal(err)
				}
			}
			_, err := script.Exec("sudo timedatectl set-timezone Asia/Shanghai").Stdout()
			if err != nil {
				logger.Sugar.Infoln("时区设置失败")
			} else {
				logger.Sugar.Infof("chrony配置文件:%s", ntpConf)
				script.File(ntpConf).Stdout()
				logger.Sugar.Infoln("chrony同步状态:")
				script.Exec("chronyc sources -v").Stdout()
				logger.Sugar.Infoln("timedatectl status:")
				script.Exec("timedatectl status").Stdout()
			}
		}
	} else {
		ntpConf := "/etc/chrony/chrony.conf"
		ntpConfPath := content.DebianChronyConf
		if !utils.TryCommand("chronyd") {
			_, err := script.Exec("sudo apt-get install -y chrony").Stdout()
			if err != nil {
				logger.Sugar.Fatal(err)
			}
		} else {
			logger.Sugar.Infoln("检测到chrony服务已存在，开始设置chrony服务")
		}
		if utils.PathExists(ntpConf) {
			script.Echo(ntpConfPath).WriteFile(ntpConf)
		} else {
			logger.Sugar.Fatalf("配置文件%s不存在，请检查chrony服务", ntpConf)
		}
		_, _ = script.Exec("sudo systemctl restart chrony").Stdout()
		_, err := script.Exec("sudo systemctl enable chrony --now").Stdout()
		if err != nil {
			logger.Sugar.Fatal(err)
		}
		_, err = script.Exec("sudo timedatectl set-timezone Asia/Shanghai").Stdout()
		if err != nil {
			logger.Sugar.Infoln("时区设置失败")
		} else {
			logger.Sugar.Infof("chrony配置文件:%s", ntpConf)
			script.File(ntpConf).Stdout()
			logger.Sugar.Infoln("chrony同步状态:")
			script.Exec("chronyc sources -v").Stdout()
			script.Exec("timedatectl status").Stdout()
		}
	}
}
func getRepo(osInfo *os.Data) {
	if osInfo.IsLikeFedora() {
		logger.Sugar.Infoln("开始更新YUM源")
	} else if osInfo.IsLikeDebian() {
		logger.Sugar.Infoln("开始更新APT源")
	}
	if osInfo.IsCentOS() {
		if osInfo.IsCentOS7() {
			logger.Sugar.Infoln("curl -fsSL -o /etc/yum.repos.d/CentOS-Base.repo https://mirrors.cloud.tencent.com/repo/centos7_base.repo")
			_, err := script.Exec("curl -fsSL -o /etc/yum.repos.d/CentOS-Base.repo https://mirrors.cloud.tencent.com/repo/centos7_base.repo").Stdout()
			if err != nil {
				logger.Sugar.Fatal(err)
			}
			logger.Sugar.Infoln("curl -fsSL -o /etc/yum.repos.d/epel.repo https://mirrors.cloud.tencent.com/repo/epel-7.repo")
			_, err = script.Exec("curl -fsSL -o /etc/yum.repos.d/epel.repo https://mirrors.cloud.tencent.com/repo/epel-7.repo").Stdout()
			if err != nil {
				logger.Sugar.Fatal(err)
			}
		} else if osInfo.IsCentOS8() {
			_ = exec.Command("/bin/bash", "-c", "rm -rf /etc/yum.repos.d/*.repo").Run()
			time.Sleep(time.Duration(1) * time.Second)
			_, err := script.Echo(content.Centos8BaseRepo).WriteFile("/etc/yum.repos.d/CentOS-Base.repo")
			if err != nil {
				logger.Sugar.Fatal(err)
			}
			_, err = script.Echo(content.Centos8EpelRepo).WriteFile("/etc/yum.repos.d/CentOS-Epel.repo")
			if err != nil {
				logger.Sugar.Fatal(err)
			}
			_, err = script.Echo(content.Centos8AppStreamRepo).WriteFile("/etc/yum.repos.d/CentOS-Linux-AppStream.repo")
			if err != nil {
				logger.Sugar.Fatal(err)
			}
			_, err = script.Echo(content.Centos8EpelKey).WriteFile("/etc/pki/rpm-gpg/RPM-GPG-KEY-EPEL-8")
			if err != nil {
				logger.Sugar.Fatal(err)
			}
		} else if osInfo.IsCentOS6() {
			logger.Sugar.Infoln("curl -fsSL -o /etc/yum.repos.d/CentOS-Base.repo https://mirrors.cloud.tencent.com/repo/centos6_base.repo")
			_, err := script.Exec("curl -fsSL -o /etc/yum.repos.d/CentOS-Base.repo https://mirrors.cloud.tencent.com/repo/centos6_base.repo").Stdout()
			if err != nil {
				logger.Sugar.Fatal(err)
			}
			logger.Sugar.Infoln("curl -fsSL -o /etc/yum.repos.d/epel.repo https://mirrors.cloud.tencent.com/repo/epel-6.repo")
			_, err = script.Exec("curl -fsSL -o /etc/yum.repos.d/epel.repo https://mirrors.cloud.tencent.com/repo/epel-6.repo").Stdout()
			if err != nil {
				logger.Sugar.Fatal(err)
			}
		}

		logger.Sugar.Infoln("yum clean all:")
		script.Exec("sudo yum clean all").Stdout()
		logger.Sugar.Infoln("yum makecache生成缓存:")
		_, err := script.Exec("sudo yum makecache").Stdout()
		if err != nil {
			logger.Sugar.Fatalf("更新YUM源失败:%s", err)
		} else if osInfo.IsCentOS6() && !osInfo.IsLikeDebian() {
			logger.Sugar.Infoln("当前yum repolist:")
			script.Exec("yum repolist").Stdout()
			logger.Sugar.Infoln("更新YUM源成功")
		} else if osInfo.IsCentOS7() {
			p := script.Exec("sudo yum install yum-complete-transaction -y")
			p.Wait()
			script.Exec("sudo yum-complete-transaction --cleanup-only").Stdout()
			logger.Sugar.Infoln("当前yum repolist:")
			script.Exec("yum repolist").Stdout()
			logger.Sugar.Infoln("更新YUM源成功")
		}
	} else if osInfo.IsLikeDebian() {
		nickName, err := script.Exec("lsb_release -cs").First(1).String()
		osNickName := strings.Split(nickName, "\n")[0]
		if err != nil {
			logger.Sugar.Fatal("获取lsb_release失败，安装退出")
		}
		_, err = script.Echo(content.AptSourceConf).WriteFile("/etc/apt/sources.list")
		if err == nil {
			_ = utils.Replace("/etc/apt/sources.list", "lsb_release", osNickName)
		}
		logger.Sugar.Infoln("apt-get update:")
		_, err = script.Exec("sudo apt-get update").Stdout()
		if err != nil {
			logger.Sugar.Fatalf("更新APT源失败:%s", err)
		} else {
			script.File("/etc/apt/sources.list").Concat().Stdout()
			logger.Sugar.Infoln("更新APT源成功")
		}
	}
}

func updatePkg(osInfo *os.Data) {
	getRepo(osInfo)
	logger.Sugar.Infoln("检查安装常用工具软件")
	pkgs := []string{"wget", "curl", "iftop", "rsync", "telnet", "jq", "git", "unzip", "net-tools", "lrzsz", "bash-completion", "sysstat", "chrony", "nc", "tcpdump"}
	logger.Sugar.Infoln("检查及安装:", pkgs)
	for i := 0; i < len(pkgs); i++ {
		if utils.TryCommand(pkgs[i]) {
			logger.Sugar.Infoln("command is exists:", pkgs[i])
			continue
		} else if utils.TryCommand("yum") && !osInfo.IsCentOS8() {
			logger.Sugar.Infof("开始安装%s:", pkgs[i])
			_, err := script.Exec("sudo yum -y -q install " + pkgs[i]).Stdout()
			if err != nil {
				logger.Sugar.Infoln("install failed", pkgs[i])
			}
		} else if utils.TryCommand("apt-get") {
			logger.Sugar.Infof("开始安装%s:", pkgs[i])
			_, err := script.Exec("sudo apt-get -y install " + pkgs[i]).Stdout()
			if err != nil {
				logger.Sugar.Infoln("install failed", pkgs[i])
			}
		} else if utils.TryCommand("dnf") || osInfo.IsCentOS8() {
			logger.Sugar.Infof("开始安装%s:", pkgs[i])
			_, err := script.Exec("sudo dnf -y -q install --nogpgcheck " + pkgs[i]).Stdout()
			if err != nil {
				logger.Sugar.Infoln("install failed", pkgs[i])
			}
		} else {
			logger.Sugar.Infoln("no", pkgs[i], "command found and can not be installed by neither yum,dnf nor apt-get")
		}
	}
	if osInfo.IsCentOS8() && !utils.TryCommand("python2") {
		script.Exec("sudo yum install -y python2").Stdout()
	} else if osInfo.IsLikeDebian() && !utils.TryCommand("python2") {
		script.Exec("sudo apt-get install -y python2").Stdout()
	}
	//兼容ubuntu18/20/22, centos8创建python2软链接
	if utils.TryCommand("python2") && !utils.TryCommand("python") {
		lnPython2Cmd := "ln -s /bin/python2.7 /bin/python"
		if utils.PathExists("/bin/python2.7") {
			logger.Sugar.Infof("OS未检测到python,创建python2软链接:%s", lnPython2Cmd)
			script.Exec(lnPython2Cmd).Stdout()
		}
	}

	if osInfo.IsLikeFedora() {
		logger.Sugar.Infoln("yum clean all:")
		script.Exec("sudo yum clean all")
	} else if osInfo.IsLikeDebian() {
		script.Exec("sudo apt-get autoremove -y").Stdout()
		script.Exec("sudo apt-get autoclean -y").Stdout()
	}
}

func checkMtu() (dockermtu int) {
	logger.Sugar.Infoln("检查系统网卡MTU值")
	n, _ := net.Interfaces()
	for i := 0; i < len(n); i++ {
		if utils.ContainsI(n[i].Name, "ens") || utils.ContainsI(n[i].Name, "eth") || utils.ContainsI(n[i].Name, "enp") || utils.ContainsI(n[i].Name, "wlp") {
			dockermtu = n[i].MTU
			logger.Sugar.Infoln("成功获取网卡MTU:", dockermtu)
			break
		}
	}
	if dockermtu == 0 {
		logger.Sugar.Infoln("获取网卡mtu失败，使用默认值: 1500")
		dockermtu = 1500
	}
	return
}

// 默认使用腾讯docker源安装, centos6.X使用YUM RPM安装
// 默认安装版本: default:20.10.16, centos6.X:1.7.1;
func installDocker(osInfo *os.Data) {
	logger.Sugar.Infof("开始在%s%s系统安装docker", osInfo.ID, osInfo.VersionID)
	_ = utils.MustMakeDir("/etc/docker/")
	_ = utils.MustMakeDir("/www/docker/")
	time.Sleep(time.Duration(1) * time.Second)
	dockerVersion := "20.10.16"
	mtu := checkMtu()
	dockerConf := fmt.Sprintf(`{
    "mtu": %d,
    "bip": "10.254.0.1/16",
    "registry-mirrors": [
      "https://hub-mirror.c.163.com",
      "https://mirror.baidubce.com"
    ],
    "data-root": "/www/docker",
    "exec-opts": ["native.cgroupdriver=systemd"],
    "log-level": "info",
    "log-driver": "json-file",
    "log-opts": {
        "max-size": "1024m",
        "max-file":"5"
    },
    "max-concurrent-downloads": 10,
    "max-concurrent-uploads": 10,
    "storage-driver": "overlay2",
    "storage-opts": [
        "overlay2.override_kernel_check=true"
    ]
}`, mtu)
	_, err := script.Echo(dockerConf).WriteFile("/etc/docker/daemon.json")
	if err != nil {
		logger.Sugar.Fatal("写入docker配置文件失败，请检查")
	}
	if osInfo.IsLikeFedora() && !osInfo.IsCentOS6() {
		_, _ = script.Exec("sudo yum install -y yum-utils").Stdout()
		_, _ = script.Exec("sudo yum-config-manager --add-repo https://mirrors.cloud.tencent.com/docker-ce/linux/centos/docker-ce.repo").Stdout()
		_, err := script.Exec("sudo yum install -y docker-ce-" + dockerVersion).Stdout()
		if err != nil {
			logger.Sugar.Fatalf("安装docker-%s失败:%s", dockerVersion, err)
		} else {
			logger.Sugar.Infoln("安装docker成功", dockerVersion)
		}
	} else if osInfo.IsLikeDebian() {
		nickName, err := script.Exec("lsb_release -cs").First(1).String()
		if err != nil {
			logger.Sugar.Fatalf("获取%s%s版本代号失败,docker安装退出:%s", osInfo.ID, osInfo.VersionID, err)

		}
		osNickName := strings.Split(nickName, "\n")[0]
		dockerRepoConf := fmt.Sprintf(`deb [arch=amd64] https://mirrors.cloud.tencent.com/docker-ce/linux/ubuntu %s stable
`, osNickName)
		_, _ = script.Exec("sudo apt-get install -y apt-transport-https ca-certificates curl software-properties-common").Stdout()
		_ = exec.Command("/bin/bash", "-c", "curl -fsSL  https://mirrors.cloud.tencent.com/docker-ce/linux/ubuntu/gpg | apt-key add -").Run()
		_, _ = script.Echo(dockerRepoConf).WriteFile("/etc/apt/sources.list.d/docker.list")
		logger.Sugar.Infoln("apt-get update:")
		_, _ = script.Exec("sudo apt-get update").Stdout()
		//logger.Sugar.Infof("apt-get install -y docker-ce=5:20.10.16~3-0~ubuntu-%s:", osNickName)
		//script.Exec("sudo apt-get install -y docker-ce=5:20.10.16~3-0~ubuntu-" + osNickName).Stdout()
		logger.Sugar.Infof("apt-get install -y docker-ce")
		_, _ = script.Exec("sudo apt-get install -y docker-ce").Stdout()
		//修复swap limit警告，参考https://docs.docker.com/engine/install/linux-postinstall/
		_ = utils.Replace("/etc/default/grub", "GRUB_CMDLINE_LINUX=\"\"", "GRUB_CMDLINE_LINUX=\"cgroup_enable=memory swapaccount=1\"")
		_ = exec.Command("/bin/bash", "-c", "update-grub && apt autoremove -y && apt autoclean -y").Run()
	}
	if osInfo.IsCentOS6() {
		_, _ = script.Exec("yum install -y https://get.docker.com/rpm/1.7.1/centos-6/RPMS/x86_64/docker-engine-1.7.1-1.el6.x86_64.rpm").Stdout()
		_, _ = script.Exec("sudo chkconfig docker on").Stdout()
		//docker1.7配置文件:/etc/sysconfig/docker
		err := utils.Replace("/etc/sysconfig/docker", "other_args=\"\"", "other_args=\"--graph=/www/docker\"")
		if err != nil {
			logger.Sugar.Fatal(err)
		}
		_, err = script.Exec("sudo /etc/init.d/docker restart").Stdout()
		if err != nil {
			logger.Sugar.Fatal("启动docker服务失败,安装docker退出", err)
		} else {
			logger.Sugar.Infoln("docker info:")
			_, _ = script.Exec("sudo docker info").Stdout()
		}
	} else {
		p := script.Exec("sudo systemctl restart docker")
		p.Wait()
		_, err := script.Exec("sudo systemctl enable docker --now").Stdout()
		if err != nil {
			logger.Sugar.Fatal("启动docker服务失败,安装docker退出", err)
		} else {
			logger.Sugar.Infoln("docker info:")
			_, _ = script.Exec("sudo docker info").Stdout()
		}
	}
}

func checkUserPermission() {
	if nos.Getuid() != 0 {
		logger.Sugar.Fatal("权限不足,请使用root用户执行")
	}
}

func disableUbuntuAutoUpgrade(osInfo *os.Data) {
	if osInfo.IsUbuntu() {
		_ = utils.Replace("/etc/apt/apt.conf.d/20auto-upgrades", "1", "0")
	}
}

func enableUbuntuAutoUpgrade(osInfo *os.Data) {
	if osInfo.IsUbuntu() {
		_ = utils.Replace("/etc/apt/apt.conf.d/20auto-upgrades", "0", "1")
	}
}
