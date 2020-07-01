package main

/*
Package main
用于对指定目录（会遍历子目录）进行文件上传
输入为目录dir和起始时间t
如果文件的创建时间晚于t则直接上传（目录则用来判断是否进行遍历）
如果文件的创建时间早于t但修改时间晚于t也直接上传/遍历
*/

import (
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tencentyun/cos-go-sdk-v5"
)

const sizeMB = 1024 * 1024
const osType = runtime.GOOS

type fileUploadInfo struct {
	filePath string
	keyPath  string
}

type userRuntime struct {
	cfgfile      string
	cfg          *CosSaverCfg
	skippedDir   map[string]int
	wg           sync.WaitGroup
	totalFileCnt int64
	failedFile   chan fileUploadInfo
}

var g_runtime userRuntime

// printCallerName
func printCallerName() string {
	funcName, file, line, ok := runtime.Caller(1)
	var result string
	if ok {
		result = fmt.Sprintf("file: %s, line: %d, func: %s\n", file, line, runtime.FuncForPC(funcName).Name())
	}
	//pc, _, _, _ := runtime.Caller(0)
	//return runtime.FuncForPC(pc).Name()
	return result
}

// listAllBuckets
func listAllBuckets() error {
	c := cos.NewClient(nil, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  g_runtime.cfg.SecretID,
			SecretKey: g_runtime.cfg.SercretKey,
		},
	})

	s, _, err := c.Service.Get(context.Background())
	if err != nil {
		log.Println(err)
		return err
	}

	log.Println(printCallerName())
	for _, b := range s.Buckets {
		log.Println("%#v\n", b)
	}
	return nil
}

// listItems
func listItems(c *cos.Client, delimiter string) error {
	opt := &cos.BucketGetOptions{
		Delimiter: delimiter,
		MaxKeys:   1000,
	}
	v, _, err := c.Bucket.Get(context.Background(), opt)
	if err != nil {
		log.Println(err)
		return err
	}

	log.Println(printCallerName())
	for _, c := range v.Contents {
		log.Printf("%s, %.2fMB\n", c.Key, float32(c.Size)/sizeMB)
	}
	for _, c := range v.CommonPrefixes {
		log.Printf("%s\n", c)
	}
	return nil
}

// 如果是win的路径格式则默认不会按目录上传
// D:\data\图片\2019\IMG_20190925_161110.jpg  ==》 D:\data\图片\2019\IMG_20190925_161110.jpg
// abc//def//a.jpg ==> dir-abc/ dir-def/ a.jpg
func uploadFile(filePath, keyPath string, client *cos.Client) error {

	f, err := os.Open(filePath)
	if err != nil {
		log.Println(err)
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		log.Println(err)
		return err
	}
	key := keyPath + "/" + info.Name()

	opt := &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			ContentType: "text/html",
		},
		ACLHeaderOptions: &cos.ACLHeaderOptions{
			// 如果不是必要操作，建议上传文件时不要给单个文件设置权限，避免达到限制。若不设置默认继承桶的权限。
			XCosACL: "private",
		},
	}
	// 如果想让文件按目录传输，直接配置目录到key里即可，比如：DIR1/DIR2/ItemName
	// 它就自动会按类似目录结构组织
	_, err = client.Object.Put(context.Background(), key, f, opt)
	if err != nil {
		g_runtime.failedFile <- fileUploadInfo{filePath: filePath, keyPath: keyPath}
		log.Println(err)
		return err
	}
	return nil
}

// 如果是新创建的那么直接上传
// 如果创建时间早但更新过也上传（拷贝的文件，新创建是拷贝的时间，但修改时间不变）
func isItemModified(item os.FileInfo, lastModifiedTime time.Time) bool {
	var tCreate int64
	if osType == "windows" {
		wFileSys := item.Sys().(*syscall.Win32FileAttributeData)
		tCreate = wFileSys.CreationTime.Nanoseconds()
	} else if osType == "linux" {
		//statT := item.Sys().(*syscall.Stat_t)
		//tCreate = int64(statT.Ctime.Sec)
	}

	return tCreate > lastModifiedTime.UnixNano() || item.ModTime().After(lastModifiedTime)
}

// uploadDir 遍历并上传目录里的文件，同时根据文件夹的最近修改时间判断只遍历改动时间大于last
func uploadDir(dir, keyDir string, lastModifiedTime time.Time, client *cos.Client) error {
	defer g_runtime.wg.Done()
	//defer func() { g_runtime.wgLimit-- }()

	rd, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Println(err)
		return err
	}

	log.Println(printCallerName())
	log.Println(lastModifiedTime)

	cnt := 0
	for _, item := range rd {
		if isItemModified(item, lastModifiedTime) {
			if item.IsDir() {
				_, ok := g_runtime.skippedDir[keyDir+"/"+item.Name()]
				if ok {
					log.Println("SkippedDir: " + item.Name())
				} else {
					g_runtime.wg.Add(1)

					log.Println("UploadDir: " + item.Name())
					go uploadDir(dir+"\\"+item.Name(), keyDir+"/"+item.Name(), lastModifiedTime, client)
				}

			} else {
				cnt++
				//atomic.AddInt64(&g_runtime.totalFileCnt, 1)
				log.Println("UploadFile: " + item.Name())
				uploadFile(dir+"/"+item.Name(), keyDir, client)
			}
		} else {
			log.Println("ItemNotUpdated: " + item.Name())
		}
	}

	log.Printf("%s %d\n", keyDir, cnt)
	return nil
}

func loadUploadTime() time.Time {
	defaultTime, _ := time.Parse("2006-01-02 15:04:05", g_runtime.cfg.DefaultUploadTime)

	data, err := ioutil.ReadFile(g_runtime.cfg.SourceDir + "\\lastUploadTime")
	if err != nil {
		log.Println(err)
		return defaultTime
	}
	log.Println("[lastUploadTime: " + string(data) + "]")
	t, err := time.Parse("2006-01-02 15:04:05", string(data))
	if err != nil {
		log.Println(err)
		return defaultTime
	}
	return t
}

// flushUploadTime 刷新本地的最新上传文件时间
func flushUploadTime() {
	err := ioutil.WriteFile(g_runtime.cfg.SourceDir+"/lastUploadTime", []byte(time.Now().Format("2006-01-02 15:04:05")), 0666)
	if err != nil {
		log.Fatal(err)
	}
}

func flushFailedList(failedFiles []fileUploadInfo) error {
	outputFile, outputError := os.OpenFile(g_runtime.cfg.SourceDir+"/uploadFailedFiles", os.O_WRONLY|os.O_CREATE, 0666)
	if outputError != nil {
		log.Printf("An error occurred with file opening or creation\n")
		return outputError
	}
	defer outputFile.Close()
	outputWriter := bufio.NewWriter(outputFile)
	for _, item := range failedFiles {
		outputString := item.filePath + "," + item.keyPath + "\n"
		outputWriter.WriteString(outputString)
	}
	err := outputWriter.Flush()
	if err != nil {
		log.Println("Succeeded flush file names into uploadFailedFiles")
	}
	return err
}

func loadFailedFiles() []fileUploadInfo {
	failedFiles := []fileUploadInfo{}
	data, err := ioutil.ReadFile(g_runtime.cfg.SourceDir + "\\uploadFailedFiles")
	if err != nil {
		log.Println(err)
		return failedFiles
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		failedFile := strings.Split(line, ",")
		if len(failedFile) == 2 {
			failedFiles = append(failedFiles, fileUploadInfo{filePath: failedFile[0], keyPath: failedFile[1]})
		}
	}
	return failedFiles
}

func main() {
	log.SetFlags(log.Ldate | log.Lshortfile)
	path, err := os.Executable()
	if err != nil {
		log.Println(err)
		return
	}

	cfgfile := filepath.Dir(path) + "/cosSaver.json"
	log.Println("ExePath: " + path + " CfgPath: " + cfgfile)

	cfg, err := loadConfig(cfgfile)
	if err != nil {
		log.Println("loadConfig fail: ", err)
		return
	}

	log.Println(cfg)
	g_runtime.skippedDir = make(map[string]int)
	for _, item := range cfg.SkippedDir {
		g_runtime.skippedDir[item] = 0
	}
	g_runtime.cfg = cfg

	u, _ := url.Parse(cfg.BucketURL)
	b := &cos.BaseURL{BucketURL: u}
	c := cos.NewClient(b, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  cfg.SecretID,
			SecretKey: cfg.SercretKey,
		},
	})

	uploadTime := time.Now().AddDate(0, 0, -1)
	uploadTime = loadUploadTime()

	g_runtime.failedFile = make(chan fileUploadInfo)

	failedFiles := []fileUploadInfo{}
	go func() {
		for data := range g_runtime.failedFile {
			log.Printf("FailedItem: %v\n", data)
			failedFiles = append(failedFiles, data)
		}
	}()

	if g_runtime.cfg.Action == "ALL" {
		g_runtime.wg.Add(1)
		uploadDir(g_runtime.cfg.SourceDir, g_runtime.cfg.InitKeyDir, uploadTime, c)
		g_runtime.wg.Wait()
	} else if g_runtime.cfg.Action == "FAILED" {
		filesToUpload := loadFailedFiles()
		for _, item := range filesToUpload {
			uploadFile(item.filePath, item.keyPath, c)
		}
	} else {
		log.Fatalln("Wrong Action exit!")
	}

	flushUploadTime()
	flushFailedList(failedFiles)

	log.Printf("TotalFileCnt: %d\n", g_runtime.totalFileCnt)
}
