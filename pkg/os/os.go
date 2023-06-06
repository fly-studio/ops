package os

import (
	"bufio"
	"bytes"
	"github.com/pkg/errors"
	"log"
	"os"
	"stkey/pkg/script"
	"stkey/utils"
	"strings"
)

const (
	// EtcOsRelease The path of os-release file;Centos 6.x is /etc/issue
	EtcOsRelease   = "/etc/os-release"
	IssueOsRelease = "/etc/issue"
	// DebianID [OSName]ID is the identifier used by the OS operating system.
	DebianID = "debian"
	FedoraID = "fedora"
	UbuntuID = "ubuntu"
	RhelID   = "rhel"
	CentosID = "centos"
)

// GetReleaseFile Check os release file
func GetReleaseFile(etcFile, issueFile string) (s string, err error) {
	s, err = script.File(etcFile).String()
	if err != nil {
		s, err = script.File(issueFile).String()
		return s, err
	}
	return s, err
}

// Data exposes the most common identification parameters.
type Data struct {
	ID         string
	IDLike     string
	Name       string
	PrettyName string
	Version    string
	VersionID  string
	HostName   string
	FileMap    map[string]string
}

// Parse is to parse a os release file content.
func Parse(EtcOsRelease, IssueOsRelease string) (data *Data) {
	content, err := GetReleaseFile(EtcOsRelease, IssueOsRelease)
	if err != nil {
		log.Fatal(err)
	}
	lineNumber, err := script.Echo(content).CountLines()
	if err != nil {
		return
	}
	//lineNumber =strconv.Itoa(lineNmuber)
	data = new(Data)
	switch lineNumber {
	case 2, 3:
		{
			content, err := script.Echo(content).First(1).String()
			if err != nil {
				return
			}
			content = strings.Replace(content, "\n", "", -1)

			idContent, _ := script.Echo(content).Column(1).String()
			data.ID, _ = script.Echo(idContent).Exec("tr A-Z a-z").String()
			versionId, _ := script.Echo(content).Column(3).String()
			data.VersionID, _ = script.Echo(versionId).First(1).String()
			data.HostName, _ = os.Hostname()
		}
	default:
		{
			lines, err := parseString(content)
			if err != nil {
				return
			}
			info := make(map[string]string)
			for _, v := range lines {
				key, value, err := parseLine(v)
				if err == nil {
					info[key] = value
				}
			}
			data.ID = info["ID"]
			data.IDLike = info["ID_LIKE"]
			data.Name = info["NAME"]
			data.PrettyName = info["PRETTY_NAME"]
			data.Version = info["VERSION"]
			data.VersionID = info["VERSION_ID"]
			data.HostName, _ = os.Hostname()
		}
	}
	data.FileMap = data.GetKernelFile()
	return
}

func parseString(content string) (lines []string, err error) {
	in := bytes.NewBufferString(content)
	reader := bufio.NewReader(in)
	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

func parseLine(line string) (string, string, error) {
	// skip empty lines
	if line == "" {
		return "", "", errors.New("Skipping: zero-length")
	}

	// skip comments
	if line[0] == '#' {
		return "", "", errors.New("Skipping: comment")
	}

	// try to split string at the first '='
	splitString := strings.SplitN(line, "=", 2)
	if len(splitString) != 2 {
		return "", "", errors.New("Can not extract key=value")
	}

	// trim white space from key and value
	key := splitString[0]
	key = strings.Trim(key, " ")
	value := splitString[1]
	value = strings.Trim(value, " ")

	// Handle double quotes
	if strings.ContainsAny(value, `"`) {
		first := value[0:1]
		last := value[len(value)-1:]

		if first == last && strings.ContainsAny(first, `"'`) {
			value = strings.TrimPrefix(value, `'`)
			value = strings.TrimPrefix(value, `"`)
			value = strings.TrimSuffix(value, `'`)
			value = strings.TrimSuffix(value, `"`)
		}
	}

	// expand anything else that could be escaped
	value = strings.Replace(value, `\"`, `"`, -1)
	value = strings.Replace(value, `\$`, `$`, -1)
	value = strings.Replace(value, `\\`, `\`, -1)
	value = strings.Replace(value, "\\`", "`", -1)
	value = strings.TrimRight(value, "\r\n")
	value = strings.TrimLeft(value, "\"")
	value = strings.TrimRight(value, "\"")
	return key, value, nil
}

// IsLikeDebian will return true for Debian and any other related OS, such as Ubuntu.
func (d *Data) IsLikeDebian() bool {
	return d.ID == DebianID || strings.Contains(d.IDLike, DebianID) || d.IsUbuntu()
}

// IsLikeFedora will return true for Fedora and any other related OS, such as CentOS or RHEL.
func (d *Data) IsLikeFedora() bool {
	return d.ID == FedoraID || strings.Contains(d.IDLike, FedoraID) || d.IsCentOS()
}

// IsUbuntu will return true for Ubuntu OS.
func (d *Data) IsUbuntu() bool {
	return d.ID == UbuntuID || utils.ContainsI(d.ID, UbuntuID)
}

// IsRHEL will return true for RHEL OS.
func (d *Data) IsRHEL() bool {
	return d.ID == RhelID
}

// IsCentOS will return true for CentOS.
func (d *Data) IsCentOS() bool {
	return d.ID == CentosID || utils.ContainsI(d.ID, CentosID)
}

func (d *Data) IsCentOS6() bool {
	return d.IsCentOS() && utils.ContainsI(d.VersionID, "6.")
}

func (d *Data) IsCentOS7() bool {
	return d.IsCentOS() && d.VersionID == "7"
}

func (d *Data) IsCentOS8() bool {
	return d.IsCentOS() && d.VersionID == "8"
}

func (d *Data) IsUbuntu18() bool {
	return d.IsUbuntu() && utils.ContainsI(d.VersionID, "18.")
}

func (d *Data) IsUbuntu16() bool {
	return d.IsUbuntu() && utils.ContainsI(d.VersionID, "16.")
}
func (d *Data) IsUbuntu20() bool {
	return d.IsUbuntu() && utils.ContainsI(d.VersionID, "20.")
}

func (d *Data) IsUbuntu22() bool {
	return d.IsUbuntu() && utils.ContainsI(d.VersionID, "22.")
}

func (d *Data) GetKernelFile() map[string]string {
	filePath := map[string]string{}
	if d.IsCentOS() || d.IsLikeFedora() {
		filePath["modulePath"] = "/etc/sysconfig/modules/modules.conf"
		filePath["rcLocalPath"] = "/etc/rc.d/rc.local"
		filePath["bashrcPath"] = "/etc/bashrc"
	}
	if d.IsUbuntu() || d.IsLikeDebian() {
		filePath["modulePath"] = "/etc/modules-load.d/modules.conf" //load modules
		filePath["rcLocalPath"] = "/etc/rc.local"                   //startup
		filePath["bashrcPath"] = "/etc/bash.bashrc"                 //command history

	}
	if d.IsCentOS6() {
		filePath["timeConfPath"] = ""
	} else if d.IsCentOS7() || d.IsCentOS8() {
		filePath["timeConfPath"] = "/etc/chrony.conf"
	} else if d.IsUbuntu() {
		filePath["timeConfPath"] = "/etc/chrony/chrony.conf"
	} else {
		filePath["timeConfPath"] = "/etc/chrony/chrony.conf"
	}
	filePath["bkpackagePath"] = "/tmp/bkpackage.txt"                     //bk package
	filePath["bknodemanDLPath"] = "/data/bkce/public/bknodeman/download" //download bknodeman
	filePath["s3Path"] = "/tmp/s3tmp.txt"                                //s3 cmd tmpfile
	filePath["sysctlPath"] = "/etc/sysctl.conf"
	return filePath
}
