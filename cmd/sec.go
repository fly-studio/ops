package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
	"github.com/spf13/cobra"
	"os/user"
	"runtime"
	"stkey/pkg/logger"
	"stkey/pkg/script"
	"stkey/utils"
	"strings"
)

func secDetect() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "detect-shell",
		Aliases: []string{"detect", "d"},
		Short:   "检测反弹shell等",
		Long: `Example:
sk sec detect-shell -h
`,
		Run: func(cmd *cobra.Command, args []string) {
			kill, _ := cmd.Flags().GetBool("kill")
			var det *Detect
			if kill {
				for {
					det.detect(kill)
				}
			} else {
				det.detect(kill)
			}
		},
	}
	cmd.Flags().BoolP("kill", "k", false, "后台运行并Kill进程,默认false,请谨慎使用")

	return cmd
}

func secCmd() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "sec",
		Short: "安全相关工具",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if runtime.GOOS == "windows" {
				logger.Sugar.Fatalf("不支持Windows系统")
			}
		},
	}

	cmd.AddCommand(secDetect())

	return cmd
}

type Detect struct {
	Uid         string               `json:"uid,omitempty"`
	Net         []net.ConnectionStat `json:"net,omitempty"`
	Username    string               `json:"username,omitempty"`
	Time        int64                `json:"time,omitempty"`
	LocalIP     string               `json:"local_ip,omitempty"`
	Hostname    string               `json:"hostname,omitempty"`
	ExeMd5      string               `json:"exe_md_5,omitempty"`
	Pid         int32                `json:"pid,omitempty"`
	Ppid        int32                `json:"ppid,omitempty"`
	Tgid        int32                `json:"tgid,omitempty"`
	ProcessName string               `json:"process_name,omitempty"`
	ShellType   string               `json:"shell_type,omitempty"`
	CmdLine     string               `json:"cmd_line,omitempty"`
	StdAll      string               `json:"std_all,omitempty"`
	StdIn       string               `json:"std_in,omitempty"`
	StdOut      string               `json:"std_out,omitempty"`
	StdErr      string               `json:"std_err,omitempty"`
	IsKill      bool                 `json:"is_kill,omitempty"`
}

func (d *Detect) detect(b bool) {
	data := d.Listener()
	for _, v := range data {
		if v.StdAll != "" && v.juge(v.StdAll) && !strings.Contains(v.CmdLine, "bk_gse_script") && len(v.getNet("tcp", v.Pid)) != 0 {
			v.Net = v.getNet("tcp", v.Pid)
			if b {
				r, err := v.Kill()
				if err != nil {
					//fmt.Println(err)
					v.IsKill = false
				} else {
					//fmt.Printf("kill success, pid:%v\n", v.Pid)
					v.IsKill = true
				}
				fmt.Println(r)
			}
			r := v.Pprint()
			fmt.Println(r)
		}
	}
}

func (*Detect) juge(s string) bool {
	if strings.Contains(s, `0 -> socket:`) || strings.Contains(s, `1 -> socket:`) || strings.Contains(s, `2 -> socket:`) {
		return true
	}
	return false
}

func (*Detect) getNet(kind string, pid int32) []net.ConnectionStat {
	data, err := net.ConnectionsPid(kind, pid)
	if err == nil {
		return data
	}
	return nil
}

func (d *Detect) Kill() (string, error) {
	return script.Exec("kill -9 " + fmt.Sprintf("%d", d.Pid)).String()
}

func IsThreat(pid, ppid int32) string {
	data, err := script.Exec("ls -al /proc/" + fmt.Sprintf("%d", pid) + "/fd/").String()
	if err != nil || data == "" {
		return ""
	}

	if utils.ContainsI(data, "socket") {
		return data
	}
	if utils.ContainsI(data, "pipe") {
		data, err := script.Exec("ls -al /proc/" + fmt.Sprintf("%d", ppid) + "/fd/").String()
		if err != nil || data == "" {
			return ""
		}
		if utils.ContainsI(data, "socket") {
			return data
		}
	}
	return ""
}

func (d *Detect) Pprint() string {
	b, err := json.Marshal(d)
	if err != nil {
		return fmt.Sprintf("%+v", d)
	}

	var out bytes.Buffer
	err = json.Indent(&out, b, "", "    ")
	if err != nil {
		return fmt.Sprintf("%+v", d)
	}
	return out.String()
}

func (d *Detect) Listener() []*Detect {
	Dlist := []*Detect{}
	processes, _ := process.Processes()
	for _, p := range processes {
		pname, err := p.Name()
		if err == nil {
			cmdline, _ := p.Cmdline()
			if utils.ContainsI(pname, "bash") || utils.ContainsI(pname, "/bin/sh") || d.IsBash(cmdline) || pname == "nc" || pname == "ncat" {
				tgid, _ := p.Tgid()
				username, _ := p.Username()
				time, _ := p.CreateTime()
				localip, _ := d.getHostIP()
				hostname, _ := d.getHostName()
				uid, _ := d.getUid(username)
				ppid, _ := p.Ppid()
				pid := p.Pid
				std := IsThreat(pid, ppid)
				stdin, _ := d.getStd(pid, "0")
				stdout, _ := d.getStd(pid, "1")
				stderr, _ := d.getStd(pid, "2")
				Dlist = append(Dlist, &Detect{
					Pid:         pid,
					ProcessName: pname,
					Tgid:        tgid,
					ShellType:   d.getType(cmdline),
					CmdLine:     cmdline,
					Username:    username,
					Time:        time,
					LocalIP:     localip,
					Hostname:    hostname,
					Uid:         uid,
					Ppid:        ppid,
					StdAll:      std,
					StdIn:       stdin,
					StdOut:      stdout,
					StdErr:      stderr,
				})
			}
		}
	}
	return Dlist
}

func (d *Detect) getStd(id int32, n string) (string, error) {
	s, err := script.Exec("ls -al  /proc/" + fmt.Sprintf("%d", id) + "/fd/" + n).String()
	if s == "" || err != nil {
		return "", err
	}
	return s, nil
}

func (d *Detect) getStdout() (string, error) {
	s, err := script.Exec("ls -al  /proc/" + fmt.Sprintf("%d", d.Pid) + "/fd/1").String()
	if s == "" || err != nil {
		return "", err
	}
	return s, nil
}

func (d *Detect) getStderr() (string, error) {
	s, err := script.Exec("ls -al  /proc/" + fmt.Sprintf("%d", d.Pid) + "/fd/2").String()
	if s == "" || err != nil {
		return "", err
	}
	return s, nil
}

func (d *Detect) getUid(username string) (string, error) {
	u, err := user.Lookup(username)
	return u.Uid, err
}

func (d *Detect) getHostName() (string, error) {
	h, err := script.Exec("hostname").String()
	if err != nil || h == "" {
		return "", err
	}
	h = strings.Split(h, "\n")[0]
	return h, nil
}

func (d *Detect) getHostIP() (string, error) {
	h, err := script.Exec("hostname -I").String()
	if err != nil || h == "" {
		return "", err
	}
	h = strings.Split(h, " ")[0]
	return h, nil
}

func (d *Detect) getExecMd5(cmd string) (string, error) {
	return script.Exec("md5sum " + cmd).String()
}

func (d *Detect) getType(cmdline string) string {
	switch true {
	case d.IsBash(cmdline):
		return "bash"
	case d.IsPython(cmdline):
		return "python"
	case d.IsPhp(cmdline):
		return "php"
	case d.IsPerl(cmdline):
		return "perl"
	case d.IsNc(cmdline):
		return "nc"
	}
	return ""
}

func (d *Detect) IsBash(cmdline string) bool {
	return cmdline == "sh" || cmdline == "sh -i" || utils.ContainsI(cmdline, "bash") && !utils.ContainsI(cmdline, "python") && !utils.ContainsI(cmdline, "php") && !utils.ContainsI(cmdline, "ncat") && !utils.ContainsI(cmdline, "nc")
}

func (d *Detect) IsPython(cmdline string) bool {
	return strings.Contains(cmdline, "python")
}

func (d *Detect) IsPhp(cmdline string) bool {
	return strings.Contains(cmdline, "php")
}

func (d *Detect) IsPerl(cmdline string) bool {
	return strings.Contains(cmdline, "perl")
}

func (d *Detect) IsNc(cmdline string) bool {
	return cmdline == "nc" || cmdline == "ncat"
}
