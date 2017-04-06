package main

import (
	"crypto/sha1"
	"encoding/xml"
	"fmt"
	"github.com/go-errors/errors"
	"glyme/utils"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Wechat xml struct
type WxMsg struct {
	XMLName      xml.Name `xml:"xml"`
	ToUserName   string
	FromUserName string
	CreateTime   int
	MsgType      string
	Content      string
	PicUrl       string
	MediaId      string
	Format       string
	Recognition  string
	ThunbMediaId string
	Location_X   float32
	Location_Y   float32
	Scale        int
	Label        string
	MsgId        int64
}

type WxMsgText struct {
	XMLName      xml.Name `xml:"xml"`
	ToUserName   CDataNode
	FromUserName CDataNode
	CreateTime   int
	MsgType      CDataNode
	Content      CDataNode
}

type CDataNode struct {
	Val string `xml:",cdata"`
}

/**
 * check parameter by `criterion`
 */
func checkParam(v url.Values, criterion map[string]string) error {
	for key, _ := range criterion {
		// check existence of each key
		if _, ok := v[key]; !ok {
			return errors.New(fmt.Sprintf("key '%s' does not exists", key))
		}
	}
	return nil
}

/**
 * generate reply text message
 */
func replyText(from string, to string, content string) string {
	msg := WxMsgText{
		ToUserName:   CDataNode{Val: to},
		FromUserName: CDataNode{Val: from},
		CreateTime:   int(time.Now().Unix()),
		MsgType:      CDataNode{Val: "text"},
		Content:      CDataNode{Val: content},
	}
	output, err := xml.Marshal(msg)
	if err != nil {
		log.Println("[ERROR]", err)
		return ""
	} else {
		return string(output)
	}
}

/**
 * download file from `url`, then write the content to `to`
 */
func download(url string, to string) error {
	resp, err := http.Get(url)
	if err != nil {
		return errors.Wrap(err, 1)
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrap(err, 1)
	}

	ioutil.WriteFile(to, data, 0777)
	return nil
}

/**
 * download voice given by url
 * returns voice path
 */
func get_voice(url string, to_dir string) ([]string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, errors.Wrap(err, 1)
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, 1)
	}

	// extract voice code
	var voice_code []string
	r, _ := regexp.Compile("voice_encode_fileid=\"(.+?)\"")
	for _, voice := range r.FindAllStringSubmatch(string(data), -1) {
		if len(voice) > 1 {
			voice_code = append(voice_code, voice[1])
		}
	}

	// download all the voices
	var voice_path []string
	guid := utils.GetGuid()
	for i, code := range voice_code {
		url = "http://res.wx.qq.com/voice/getvoice?mediaid=" + code
		filename := guid + strconv.Itoa(i) + ".mp3"
		go func(url string) {
			download(url, to_dir+filename)
		}(url)
		voice_path = append(voice_path, "http://118.89.22.190/voice/"+filename)
	}

	return voice_path, nil
}

/**
 * handle wechat http request
 */
func webWx(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	log.Println("[DEBUG]", "Form:", r.Form)

	// token verification
	if r.Method == "GET" {
		if err := checkParam(r.Form, map[string]string{
			"signature": "",
			"timestamp": "",
			"nonce":     "",
			"echostr":   "",
		}); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, "Error: ", err)
			log.Print("[Error]", err)
			return
		}

		signature := strings.Join(r.Form["signature"], "")
		timestamp := strings.Join(r.Form["timestamp"], "")
		nonce := strings.Join(r.Form["nonce"], "")
		echostr := strings.Join(r.Form["echostr"], "")
		token := "glymestock123alarm"

		list := []string{token, timestamp, nonce}
		sort.Strings(list)

		fmt.Println(list)
		sha1 := sha1.New()
		for _, s := range list {
			io.WriteString(sha1, s)
		}
		hashcode := fmt.Sprintf("%x", sha1.Sum(nil))

		log.Println("[DEBUG]", "Signature:", hashcode, signature)

		// return echostr if token is verified
		if hashcode == signature {
			fmt.Fprint(w, echostr)
		} else {
			fmt.Fprint(w, "")
		}
	} else if r.Method == "POST" {
		// wechat message passive reply
		body_byte, _ := ioutil.ReadAll(r.Body)
		r.Body.Close()

		var body WxMsg
		xml.Unmarshal(body_byte, &body)

		var replyMsg string

		url := body.Content
		if url == "" {
			log.Printf("[DEBUG] Wrong query: %v\n", r.Form)
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, "Wrong query")
			return
		}

		log.Println("[DEBUG]", "voice url:", url)
		voice_path, _ := get_voice(url, "/home/glyme/voice_store/")
		replyMsg = replyText(body.ToUserName, body.FromUserName, strings.Join(voice_path, "\n"))
		fmt.Fprint(w, replyMsg)
	}
}

func main() {
	// url := "http://mp.weixin.qq.com/s?__biz=MzA5NTA2NTU4Mw==&mid=2247485654&idx=1&sn=8cef8a9460c1e6f112ef93bbd3ed53e5&chksm=90444c56a733c5400bba41d30d1a47b29bfb4436b7c607bebe67008003840724de901cfa268f&mpshare=1&scene=23&srcid=0320P8Gpc03RnM1F96FYkrhA#rd"

	fs := http.FileServer(http.Dir("/home/glyme/voice_store"))
	http.Handle("/voice/", http.StripPrefix("/voice/", fs))

	http.HandleFunc("/wx", webWx)

	var err = http.ListenAndServe(":80", nil)
	if err != nil {
		log.Fatal("ListenAndServe error:", err)
	} else {
		log.Println("[DEBUG]", "Serving at :80")
	}
}
