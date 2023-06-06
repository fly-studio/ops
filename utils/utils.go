package utils

import (
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"stkey/pkg/script"
	"strings"
)

func ContainsI(a string, b string) bool {
	return strings.Contains(
		strings.ToLower(a),
		strings.ToLower(b),
	)
}

func Replace(path, from, to string) error {
	pipe := script.File(path)
	file, err := pipe.Replace(from, to).String()
	if err != nil {
		fmt.Println(path, err)
		return fmt.Errorf("error writing file %s: %w", path, err)
	}
	return os.WriteFile(path, []byte(file), 0644)
}

func AppendFileIf(path, search, source string) error {
	p, _ := script.File(path).Match(search).String()
	if len(p) == 0 {
		_, err := script.Echo(source).AppendFile(path)
		if err != nil {
			fmt.Println(path, err)
			return fmt.Errorf("error writing file %s: %w", path, err)
		}
	}
	return nil
}

// TryCommand 嘗試執行指令，如果成功則返回true。兼容centos6.X
func TryCommand(command string) bool {
	p, _ := script.Exec("\"/bin/bash\"" + " \"-c\" " + "\"command -v \"" + command).String()
	if len(p) == 0 {
		return false
	} else {
		return true
	}
}

func MustMakeDir(path string) error {
	_, err := script.IfExists(path).Echo("").Stdout()
	if err != nil {
		script.Exec("sudo mkdir -p " + path)
		//os.MkdirAll(path, 755)
		return nil
	} else {
		return nil
	}
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func PathExists(path string) bool {
	b, _ := pathExists(path)
	return b
}

// HttpGet 獲取網頁內容
func HttpGet(url string) string {
	resp, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body)
}

// DownloadFile 下載文件
func DownloadFile(filePath string, url string) error {
	// 創建臨時檔案
	out, err := os.Create(filePath + ".tmp")
	if err != nil {
		return err
	}
	defer out.Close()

	// 下載文件
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if _, err = io.Copy(out, resp.Body); err != nil {
		return err
	}

	// 重命名臨時檔案
	if err = os.Rename(filePath+".tmp", filePath); err != nil {
		return err
	}
	return nil
}

// MD5File 计算文件的MD5值
func MD5File(filepath string) string {
	f, err := os.Open(filepath)
	if err != nil {
		log.Fatal(err)
		return ""
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		log.Fatal(err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
