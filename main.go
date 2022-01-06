/*
 * @Description: 武神活跃号日常
 * @Author: benz1
 * @Date: 2021-12-29 16:10:57
 * @LastEditTime: 2022-01-06 09:35:40
 * @LastEditors: benz1
 * @Reference:
 */

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/knva/go-rocket-update/pkg/provider"
	"github.com/knva/go-rocket-update/pkg/updater"
	"github.com/tidwall/gjson"

	"github.com/gorilla/websocket"
	"github.com/robfig/cron"
	"gopkg.in/yaml.v2"
)

type User struct {
	name   string /* 姓名 */
	id     string /* id */
	token  string /* 登录凭证 */
	server int    /* 区 */
	login  string /* 所在账号 */
	inlist bool   /* 是否登陆 */
}
type LoginData struct {
	Login    string `yaml:"login"`
	Password string `yaml:"password"`
	Server   int    `yaml:"server"`
}
type Conf struct {
	Cron           string `yaml:"cron"`
	Pushplus_token string `yaml:"pushplus_token"`
	Pushtg_token   string `yaml:"pushtg_token"`
	Pushtg_chat_id string `yaml:"pushtg_chat_id"`
	Blacklist      string `yaml:"blacklist"`
	Logins         []LoginData
}

var (
	wg       sync.WaitGroup
	urls     = make(map[int]string) /* WS地址 */
	users    []User
	conf     Conf
	mode     string     /* 运行模式 */
	text     = ""       /* 推送消息 */
	lose     int        /* 失败个数 */
	succ     int        /* 成功个数 */
	loselock sync.Mutex /* 失败锁 */
	succlock sync.Mutex /* 成功锁 */
)

/**
 * @description:			通过pushplus推送
 * @param {string} token	token
 * @param {string} msg		推送信息
 * @return {*}
 */
func pushPlusNotify(token string, msg string) {
	methodName := "PUSHPLUS推送任务"
	url := "http://www.pushplus.plus/send"
	contentType := "application/json"
	data := `{"token":"` + token + `","template":"txt","title":"🔰活跃号日常推送 ","content":"` + msg + `"}`
	resp, err := http.Post(url, contentType, strings.NewReader(data))
	if err != nil {
		log4go(methodName, "ERROR").Println(err)
		return
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log4go(methodName, "ERROR").Println(err)
		return
	}
	message := struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data string `json:"data"`
	}{}
	unmarshal_err := json.Unmarshal(body, &message)
	if unmarshal_err != nil {
		log4go(methodName, "ERROR").Println(unmarshal_err)
	}
	if message.Code == 999 {
		log4go(methodName, "ERROR").Println(message.Msg)
	} else if message.Code == 200 {
		log4go(methodName, "INFO").Println(message.Msg)
	}
}

/**
 * @description:			通过TG推送
 * @param {string} token	API Token
 * @param {string} chat_id	User id
 * @param {string} msg		推送消息
 * @return {*}
 */
func pushtgNotify(token string, chat_id string, msg string) {
	methodName := "TG推送任务"
	url := "https://api.telegram.org/bot" + token + "/sendMessage"
	contentType := "application/json"
	data := `{"chat_id":"` + chat_id + `","parse_mode":"Markdown","text":"` + "🔰*活跃号日常推送* \n" + msg + `"}`
	resp, err := http.Post(url, contentType, strings.NewReader(data))
	if err != nil {
		log4go(methodName, "ERROR").Println(err)
		return
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log4go(methodName, "ERROR").Println(err)
		return
	}
	message := struct {
		OK          bool   `json:"ok"`
		ErrorCode   int    `json:"error_code"`
		Description string `json:"description"`
	}{}
	unmarshal_err := json.Unmarshal(body, &message)
	if unmarshal_err != nil {
		log4go(methodName, "ERROR").Println(unmarshal_err)
	}
	if !message.OK {
		log4go(methodName, "ERROR").Println(message.Description)
	} else if message.OK {
		log4go(methodName, "INFO").Println(message.Description)
	}
}

/**
 * @description:	检查文件是否存在
 * @param {string} filename 文件名
 * @return {bool} 是否存在
 */
func checkFileIsExist(filename string) bool {
	var exist = true
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		exist = false
	}
	return exist
}

/**
 * @description:	生成配置
 * @param {*}
 * @return {*}
 */
func newConf() {
	if checkFileIsExist("./conf.yaml") {
		return
	}

	logins := []LoginData{{
		Login: `xxx`, Password: `xxx`, Server: 1,
	}}
	var conf = Conf{
		Cron:           "0 30 6,14,22 * * *",
		Pushplus_token: ``,
		Pushtg_token:   ``,
		Pushtg_chat_id: ``,
		Blacklist:      ``,
		Logins:         logins,
	}
	str, err := yaml.Marshal(conf)
	if err != nil {
		return
	}
	err = ioutil.WriteFile("./conf.yaml", str, 0666)
	if err != nil {
		return
	}
}

/**
 * @description:	初始化配置
 * @param {*}
 * @return {*}
 */
func iniConf() {
	conf = Conf{}
	c, err := ioutil.ReadFile("./conf.yaml")
	if err != nil {
		log4go("读取配置", "ERROR").Fatalln(err)
		return
	}
	err = yaml.Unmarshal(c, &conf)
	if err != nil {
		log4go("读取配置", "ERROR").Fatalln(err)
		return
	}
	if conf.Cron == "" || len(conf.Logins) == 0 {
		log4go("读取配置", "ERROR").Fatalln(`配置文件错误,请检测`)
	} else {
		for _, login := range conf.Logins {
			if login.Login == "" || login.Password == "" || login.Server == 0 {
				log4go("读取配置", "ERROR").Fatalln(`配置文件错误,请检测`)
			}
		}
	}
	if !strings.HasSuffix(strings.TrimSpace(conf.Blacklist), ",") {
		conf.Blacklist = conf.Blacklist + ","
	}
	log4go("读取配置", "INFO").Println(`读取配置成功`)
}

/**
 * @description:            格式化打印LOG
 * @param {string} prefix	前缀
 * @param {string} message	信息
 * @return {*}
 */
func log4go(name string, msgType string) (logger *log.Logger) {
	switch msgType {
	case "INFO":
		logger = log.New(os.Stdout, "["+name+"]", log.Ldate|log.Ltime)
	case "ERROR":
		logger = log.New(os.Stdout, "["+name+" "+msgType+"]", log.Lshortfile|log.Ldate|log.Ltime)
	}
	return
}

/**
 * @description:	获取武神WS地址
 * @param {*}
 * @return {*}		数组MAP
 */
func getWsUrl() (urls map[int]string) {
	methodName := "获取WS连接"
	resp, err := http.Get("http://game.wsmud.com/game/getserver")
	if err != nil {
		log4go(methodName, "ERROR").Fatalln(err)
		return
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log4go(methodName, "ERROR").Fatalln(err)
		return
	}
	urlsJson := []struct {
		Id   int    `json:"id"`
		Port int    `json:"port"`
		Ip   string `json:"ip"`
	}{}
	unmarshal_err := json.Unmarshal(body, &urlsJson)
	if unmarshal_err != nil {
		log4go(methodName, "ERROR").Fatalln(unmarshal_err)
	}
	urls = make(map[int]string)
	for _, url := range urlsJson {
		urls[url.Id] = "ws://" + url.Ip + ":" + strconv.Itoa(url.Port)
	}
	if len(urls) == 0 {
		log4go(methodName, "ERROR").Fatalln("获取URL失败")
	}
	log4go(methodName, "INFO").Println("获取WS连接成功")
	wg.Done()
	return
}

/**
 * @description:            根据账号密码获取token
 * @param {string} login	账号
 * @param {string} password	密码
 * @return {*}
 */
func getToken(login string, password string) (token string) {
	methodName := "获取登录凭证"
	url := "http://game.wsmud.com/userapi/login"
	contentType := "application/x-www-form-urlencoded; charset=UTF-8"
	data := `code=` + login + `&pwd=` + password
	resp, err := http.Post(url, contentType, strings.NewReader(data))
	if err != nil {
		log4go(methodName+login, "ERROR").Fatalln(err)
		return
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log4go(methodName+login, "ERROR").Fatalln(err)
		return
	}
	bodyJson := struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}{}
	unmarshal_err := json.Unmarshal(body, &bodyJson)
	if unmarshal_err != nil {
		log4go(methodName+login, "ERROR").Fatalln(unmarshal_err)
	}
	if bodyJson.Code == 0 {
		log4go(methodName+login, "ERROR").Fatalln(bodyJson.Message)
	}
	if bodyJson.Code == 1 {
		cookies := resp.Cookies()
		token = cookies[0].Value + " " + cookies[1].Value
		wg.Done()
	}
	return
}

/**
 * @description:		替换切片不需要的元素为空格
 * @param {[]byte} data	输入
 * @return {*}			输出
 */
func regByte(data []byte) []byte {
	for k, v := range data {
		if v == 10 {
			data[k] = 32
		}
	}
	return data
}

/**
 * @description:		格式化json
 * @param {[]byte} Data	输入
 * @return {*}			转化后输出
 */
func regJsonData(data []byte) []byte {
	reg := regexp.MustCompile(`([a-zA-Z]\w*):`)
	newStr := reg.ReplaceAllString(string(data), `"$1":`)
	reg = regexp.MustCompile(`<cmd cmd=.*?>.*?</cmd>`)
	newStr = reg.ReplaceAllString(newStr, ``)

	newStr = strings.Replace(newStr, `\n:`, "", -1)
	newStr = strings.Replace(newStr, `\r:`, "", -1)
	newStr = strings.Replace(newStr, `\t:`, "", -1)
	newStr = strings.Replace(newStr, `'`, `"`, -1)
	newStr = strings.Replace(newStr, `,0]}`, `]}`, -1)
	return []byte(newStr)
}

/**
 * @description:            获取角色数组
 * @param {int} server		所在区
 * @param {string} token	登陆凭证
 * @return {*}
 */
func getRoles(server int, token string, login string) (users []User) {
	methodName := "获取角色"
	var header = http.Header{}
	header.Set("Origin", "http://game.wsmud.com")
	ws, _, err := websocket.DefaultDialer.Dial(urls[server], header)
	if err != nil {
		log4go(methodName+login, "ERROR").Fatalln(err)
		return
	}
	defer ws.Close()
	waitcmd(ws, token, 500)
	_, message, err := ws.ReadMessage()
	if err != nil {
		log4go(methodName+login, "ERROR").Fatalln(err)
		return
	}
	roles := struct {
		Type  string `json:"type"`
		Roles []struct {
			Name string `json:"name"`
			Id   string `json:"id"`
		}
	}{}
	unmarshal_err := json.Unmarshal(regJsonData(message), &roles)
	if unmarshal_err != nil {
		log4go(methodName+login, "ERROR").Fatalln(unmarshal_err)
	}
	if roles.Type == "roles" {
		users = make([]User, len(roles.Roles))
		for n, role := range roles.Roles {
			users[n] = User{
				name:   role.Name,
				id:     role.Id,
				token:  token,
				server: server,
				login:  login,
			}
			if strings.Contains(conf.Blacklist, role.Name+",") {
				users[n].inlist = true
			} else {
				users[n].inlist = false
			}
		}
		defer wg.Done()
	}
	return
}

/**
 * @description:                封装发送函数
 * @param {*websocket.Conn} ws  WS
 * @param {string} msg          指令
 * @return {*}
 */
func write(ws *websocket.Conn, msg string) {
	cmds := strings.Split(msg, ",")
	for _, cmd := range cmds {
		waitcmd(ws, cmd, rand.Intn(50)+250)
	}
}

/**
 * @description:                等待t毫秒后发送
 * @param {*websocket.Conn} ws  WS
 * @param {string} msg          指令
 * @param {int} t               t毫秒
 * @return {*}
 */
func waitcmd(ws *websocket.Conn, msg string, t int) {
	if msg == "close" {
		time.Sleep(time.Millisecond * time.Duration(t))
		ws.Close()
		return
	}
	cmds := strings.Split(msg, ",")
	if len(cmds) == 1 {
		time.Sleep(time.Millisecond * time.Duration(t))
		err := ws.WriteMessage(websocket.TextMessage, []byte(msg))
		if err != nil {
			log4go(msg, "ERROR").Println(err)
			return
		}
	} else {
		ti := t / len(cmds)
		for _, cmd := range cmds {
			waitcmd(ws, cmd, ti)
		}
	}
}

/**
 * @description: 		日常任务
 * @param {User} user	角色
 * @return {*}
 */
func daily(user User) {
	if user.inlist {
		log4go(user.name, "INFO").Println("黑名单已跳过")
		return
	}
	methodName := "日常任务"
	name := user.name
	id := user.id
	token := user.token
	server := user.server
	var (
		family = ""    /* 门派 */
		level  = ""    /* 等级 */
		isMe   = false /* 首席是否是自己 */
		qa     = false /* 请安是否完成 */
		zb     = false /* 追捕是否完成 */
		sm     = false /* 师门是否完成 */
		fb     = false /* 副本是否完成 */
		smover = -1    /* 剩余师门次数 */
		fbover = -1    /* 剩余副本次数 */
		zbover = -1    /* 剩余追捕次数 */
		gotoQa = false /* 开始请安 */
		gotoZb = false /* 开始追捕 */
		gotoSm = false /* 开始师门 */
		gotoFb = false /* 开始副本 */
		qaNpc  = Npc{} /* 请安NPC */
		smNpc  = Npc{} /* 师门NPC */
		buyNpc = Npc{} /* 商人NPC */
		zbNpc  = Npc{  /* 追捕NPC */
			id:   "",
			name: "扬州知府 程药发",
			way:  "扬州城-衙门正厅",
			item: "",
		}
		room      = "" /* 房间名字 */
		roomTimer = time.NewTimer(time.Second * time.Duration(120))
	)
	var header = http.Header{}
	header.Set("Origin", "http://game.wsmud.com")
	ws, _, err := websocket.DefaultDialer.Dial(urls[server], header)
	if err != nil {
		log4go(methodName, "ERROR").Println(err)
		return
	}
	defer ws.Close()
	waitcmd(ws, token, 500)
	if err != nil {
		log4go(methodName, "ERROR").Println(err)
		return
	}
Loop:
	for {
		select {
		case <-roomTimer.C:
			waitcmd(ws, "tasks", 500)
		default:
		}
		_, message, err := ws.ReadMessage()
		if err != nil {
			log4go(methodName, "ERROR").Println(err)
			break Loop
		}
		message = regByte(message)

		re := regexp.MustCompile(`^{.*}$`)
		if matched := re.MatchString(string(message)); !matched {
			message = []byte(`{type:"text",msg:"` + string(message) + `"}`)
		}
		message_str := string(regJsonData(message))
		if len(string(message)) != 0 {
			data := struct {
				Type string `json:"type"`
			}{}

			data.Type = gjson.Get(message_str, `type`).Str
			if data.Type == "roles" {
				write(ws, "login "+id)
				continue Loop
			}
			if data.Type == "login" {
				log4go(name, "INFO").Println("登陆")
				write(ws, `setting off_plist 1,setting off_move 1,setting off_move 1`)
				waitcmd(ws, "score", 500)
				continue Loop
			}
			if data.Type == "room" {
				room = gjson.Get(message_str, "name").Str
				if strings.Contains(room, `副本区域`) {
					waitcmd(ws, `cr over`, 500)
					fbover = fbover - 1
					if fbover <= 0 {
						fb = true
						waitcmd(ws, "tasks", 500)
					}
				}
				if !strings.Contains(room, `副本区域`) && gotoFb && fbover > 0 {
					waitcmd(ws, `cr yz/lw/shangu`, 500)
				}
				if !roomTimer.Stop() {
					select {
					case <-roomTimer.C:
					default:
					}
				}
				roomTimer.Reset(time.Second * time.Duration(120))
				continue Loop
			}
			if data.Type == "loginerror" {
				msg := gjson.Get(message_str, "msg").Str
				log4go(name, "ERROR").Println(msg)
				loselock.Lock()
				text = text + name + " : " + msg + `, 所在账号 ` + user.login + `\n`
				lose = lose + 1
				loselock.Unlock()
				ws.Close()
				break Loop
			}
			if data.Type == "text" {
				msg := gjson.Get(message_str, "msg").Str
				if gotoQa && strings.Contains(msg, `你要看什么`) {
					qa = true
					gotoQa = false
					waitcmd(ws, "tasks", 500)
				}
				if gotoSm {
					if strings.Contains(msg, `帮我找`) {
						re := regexp.MustCompile(`<.*?>(.*?)</.*?>`)
						result := re.FindStringSubmatch(msg)[0]
						for _, buy := range buyNpcS {
							for _, sale := range buy.sale {
								if strings.Contains(sale, result) {
									buyNpc.name = buy.name
									buyNpc.way = buy.way
									buyNpc.item = result
									write(ws, ways[buy.way])
									continue Loop
								}
							}
						}
						write(ws, `task sm `+smNpc.id)
					}
					if strings.Contains(msg, `师父让别人去找`) || strings.Contains(msg, `你的师门任务完成了`) {
						write(ws, `task sm `+smNpc.id)
					}
					if strings.Contains(msg, `辛苦了， 你先去休息一下吧`) {
						sm = true
						gotoSm = false
						waitcmd(ws, "tasks", 500)
					}
					re := regexp.MustCompile(`你的师门任务完成了，目前完成\d+/\d+个`)
					if re.MatchString(msg) {
						result := re.FindStringSubmatch(msg)
						log4go(name, "INFO").Println(result[0])
					}
				}
				if gotoZb {
					if strings.Contains(msg, `你可以接别的逃犯来继续做`) {
						write(ws, `ask3 `+zbNpc.id)
					}
					if strings.Contains(msg, `你的追捕任务完成了，目前完成20/20个`) {
						zb = true
						gotoZb = false
						waitcmd(ws, "tasks", 500)
					}
					if strings.Contains(msg, `你的追捕任务已经完成了`) || strings.Contains(msg, `最近没有在逃的逃犯`) {
						zb = true
						gotoZb = false
						waitcmd(ws, "tasks", 500)
					}
					re := regexp.MustCompile(`你的追捕任务完成了，目前完成\d+/\d+个`)
					if re.MatchString(msg) {
						result := re.FindStringSubmatch(msg)
						log4go(name, "INFO").Println(result[0])
					}
				}
				if gotoFb {
					re := regexp.MustCompile(`完成度`)
					if re.MatchString(msg) {
						log4go(name, "INFO").Println(`剩余副本次数: ` + strconv.Itoa(fbover))
						if fbover <= 0 {
							fb = true
							gotoFb = false
						}
					}
				}
				continue Loop
			}
			if data.Type == "items" {
				items := gjson.Get(message_str, `items`).Array()
				for _, item := range items {
					if item.Get(`p`).Int() == 1 {
						continue
					}
					if item.Get(`name`).Str == "" {
						continue
					}
					if gotoQa {
						if strings.Contains(item.Get(`name`).Str, qaNpc.name) {
							if strings.Contains(item.Get(`name`).Str, name) {
								isMe = true
								gotoQa = false
								waitcmd(ws, "tasks", 500)
							} else {
								write(ws, `ask2 `+item.Get(`id`).Str+`,look bi`)
							}
						}
					}
					if gotoSm {
						if strings.Contains(item.Get(`name`).Str, smNpc.name) {
							smNpc.id = item.Get(`id`).Str
							write(ws, `task sm `+smNpc.id)
						}
						if strings.Contains(item.Get(`name`).Str, buyNpc.name) {
							buyNpc.id = item.Get(`id`).Str
							write(ws, `sell all,list `+buyNpc.id)
						}
					}
					if gotoZb {
						if strings.Contains(item.Get(`name`).Str, zbNpc.name) {
							zbNpc.id = item.Get(`id`).Str
							write(ws, `ask1 `+zbNpc.id+`,ask2 `+zbNpc.id)
						}
					}
				}
				continue Loop
			}
			if data.Type == "cmds" {
				if gotoSm {
					var giveup = ""
					items := gjson.Get(message_str, `items`).Array()
					for _, item := range items {
						if strings.Contains(item.Get(`name`).Str, "放弃") {
							giveup = item.Get(`cmd`).Str
						}
						if item.Get(`name`).Str == `上交`+buyNpc.item {
							write(ws, item.Get(`cmd`).Str)
							continue Loop
						}
					}
					write(ws, giveup)
				}
				continue Loop
			}
			if data.Type == "dialog" {
				dialog := gjson.Get(message_str, `dialog`).Str
				switch dialog {
				case "tasks":
					items := gjson.Get(message_str, "items").Array()
					if len(items) != 0 {
						for _, item := range items {
							if item.Get(`state`).Int() == 2 {
								write(ws, `taskover `+item.Get(`id`).Str)
							}
							switch item.Get(`id`).Str {
							case "signin":
								if isMe {
									qa = true
								} else {
									if strings.Contains(item.Get(`desc`).Str, "还没有给首席请安") && family != "武馆" {
										qa = false
									} else {
										qa = true
									}
								}
								if item.Get(`state`).Int() == 3 {
									sm = true
									fb = true
								}
								re := regexp.MustCompile(`今日副本完成次数：(\d+)`)
								result := re.FindStringSubmatch(item.Get(`desc`).Str)
								s, _ := strconv.Atoi(result[1])
								fbover = 20 - s
								if fbover <= 0 {
									fbover = 0
									fb = true
								} else {
									fb = false
								}
							case "sm":
								if item.Get(`state`).Int() == 3 {
									sm = true
								}
								re := regexp.MustCompile(`完成(\d+)/(\d+)`)
								result := re.FindStringSubmatch(item.Get(`desc`).Str)
								s1, _ := strconv.Atoi(result[1])
								s2, _ := strconv.Atoi(result[2])
								smover = s2 - s1
							case "yamen":
								if item.Get(`state`).Int() == 3 {
									zb = true
								}
								re := regexp.MustCompile(`完成(\d+)/(\d+)个，共连续完成(\d+)`)
								result := re.FindStringSubmatch(item.Get(`desc`).Str)
								s1, _ := strconv.Atoi(result[1])
								s2, _ := strconv.Atoi(result[2])
								zbover = s2 - s1
							}
						}
						log4go(name, "INFO").Println(`请安完成情况: ` + strconv.FormatBool(qa))
						log4go(name, "INFO").Println(`师门完成情况: ` + strconv.FormatBool(sm) + `,剩余次数: ` + strconv.FormatInt(int64(smover), 10))
						log4go(name, "INFO").Println(`副本完成情况: ` + strconv.FormatBool(fb) + `,剩余次数: ` + strconv.FormatInt(int64(fbover), 10))
						log4go(name, "INFO").Println(`追捕完成情况: ` + strconv.FormatBool(zb) + `,剩余次数: ` + strconv.FormatInt(int64(zbover), 10))
						write(ws, `stopstate`)
						if !qa {
							log4go(name, "INFO").Println(`开始请安`)
							gotoQa = true
							if strings.HasPrefix(qaNpc.way, "-") {
								write(ws, strings.Replace(qaNpc.way, `-`, ``, -1))
							} else {
								write(ws, ways[qaNpc.way])
							}
						} else if !sm {
							log4go(name, "INFO").Println(`开始师门`)
							gotoSm = true
							if strings.HasPrefix(smNpc.way, "-") {
								write(ws, strings.Replace(smNpc.way, `-`, ``, -1))
							} else {
								write(ws, ways[smNpc.way])
							}
						} else if !fb {
							log4go(name, "INFO").Println(`开始副本`)
							gotoFb = true
							write(ws, `jh fam 0 start`)
						} else if !zb {
							log4go(name, "INFO").Println(`开始追捕`)
							gotoZb = true
							write(ws, `shop 0 50,`+ways[zbNpc.way])
						} else {
							succlock.Lock()
							succ = succ + 1
							succlock.Unlock()
							log4go(name, "INFO").Println(`日常任务完成`)
							if strings.Contains(level, "武帝") || strings.Contains(level, "武神") {
								write(ws, `tm 回家自闭,jh fam 0 start,go west,go west,go north,go enter,go west,xiulian`)
							} else {
								write(ws, `tm 开始挖矿,wakuang`)
							}
							waitcmd(ws, "close", 2000)
							break Loop
						}
					}
				case "score":
					family = gjson.Get(message_str, `family`).String()
					level = gjson.Get(message_str, `level`).String()
					if family == "无门无派" {
						family = "武馆"
					}
					qaNpc = qaNpcs[family]
					smNpc = smNpcs[family]
					waitcmd(ws, "tasks", 500)
				case "list":
					if gotoSm {
						selllist := gjson.Get(message_str, "selllist").Array()
						for _, item := range selllist {
							if strings.Contains(item.Get("name").Str, buyNpc.item) {
								write(ws, `buy 1 `+item.Get("id").Str+` from `+buyNpc.id+`,`+ways[smNpc.way])
							}
						}
					}
				}
				continue Loop
			}
		}
	}
}

/**
 * @description:                线程
 * @param {int} id              序号
 * @param {<-chanUser} jobs     任务
 * @param {chan<-User} result   结果
 * @return {*}
 */
func worker(id int, jobs <-chan User, result chan<- User) {
	for job := range jobs {
		daily(job)
		result <- job
	}
}

/**
 * @description:	主任务
 * @param {*}
 * @return {*}
 */
func task() {
	log4go("定时任务", "INFO").Println(`开启所有日常任务`)
	text = ""
	lose = 0
	succ = 0
	wg.Add(1)
	urls = getWsUrl()
	wg.Wait()
	log4go("定时任务", "INFO").Println(`开始获取角色`)
	users = []User{}
	for _, login := range conf.Logins {
		wg.Add(1)
		token := getToken(login.Login, login.Password)
		wg.Wait()
		wg.Add(1)
		users = append(users, getRoles(login.Server, token, login.Login)...)
		wg.Wait()
	}
	log4go("定时任务", "INFO").Println(`获取角色成功`)
	jobs := make(chan User, 10086)
	result := make(chan User, 10086)
	for i := 0; i < 30; i++ {
		go worker(1, jobs, result)
	}
	for _, user := range users {
		jobs <- user
	}
	close(jobs)
	for range users {
		<-result
	}
	text = text + `完成:` + strconv.Itoa(succ) + `个,失败:` + strconv.Itoa(lose) + `个,未知:` + strconv.Itoa(len(users)-lose-succ) + `个。\n`
	text = text + `*结束所有日常任务*\n`
	if conf.Pushplus_token != "" {
		pushPlusNotify(conf.Pushplus_token, text)
	}
	if conf.Pushtg_token != "" && conf.Pushtg_chat_id != "" {
		pushtgNotify(conf.Pushtg_token, conf.Pushtg_chat_id, text)
	}
	log4go("定时任务", "INFO").Println(`结束所有日常任务`)
}

/**
 * @description: 主函数
 * @param {*}
 * @return {*}
 */
func main() {

	u := &updater.Updater{
		Provider: &provider.Github{
			RepositoryURL: "github.com/BenZinaDaze/wsdaily",
			ArchiveName:   fmt.Sprintf("wsdaily_%s_%s.zip", runtime.GOOS, runtime.GOARCH),
		},
		ExecutableName: "wsdaily",
		Version:        "v1.06", // 注意每次更新需要更新这个版本
	}
	fmt.Printf("平台:%s_%s,版本:%s\n", runtime.GOOS, runtime.GOARCH, u.Version)
	res, err := u.Update()
	if err != nil {
		log4go("更新出错", "ERROR").Println(err)
	}
	if res == 2 {
		log4go("更新", "INFO").Println("已经更新到最新版本,请重启应用!")
		os.Exit(0)
	}

	if !checkFileIsExist("./conf.yaml") {
		newConf()
		log4go("配置文件不存在", "ERROR").Println(`已生成配置文件,请按规则配置参数,配置完成后重启应用.`)
		return
	}

	iniConf()
	flag.StringVar(&mode, "mode", "cron", "运行模式")
	flag.Parse()
	if mode == "cron" {
		cr := cron.New()
		cr.AddFunc(conf.Cron, task)
		cr.Start()
		select {}
	} else if mode == "run" {
		task()
	} else {
		log4go("参数错误", "ERROR").Println(`MODE参数设置错误`)
	}
}
