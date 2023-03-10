package template

import (
	"encoding/json"
	"fmt"
	"github.com/Xhofe/alist/conf"
	"github.com/Xhofe/alist/drivers/base"
	"github.com/Xhofe/alist/model"
	"github.com/Xhofe/alist/utils"
	"github.com/bitly/go-simplejson"
	"github.com/go-resty/resty/v2"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// write util func here, such as cal sign

var chaoxingClient = resty.New()

var form_login_fmt = "fid=-1&uname=%s&password=%s&t=true&forbidotherlogin=0&validate=&doubleFactorLogin=0"

var api_list_root = "https://pan-yz.chaoxing.com/opt/listres?page=1&size=%d&enc=%s"
var api_list_file = "https://pan-yz.chaoxing.com/opt/listres?puid=%s&shareid=%s&parentId=%s&page=1&size=%d&enc=%s"
var api_list_shared_root = "https://pan-yz.chaoxing.com/opt/listres?puid=0&shareid=-1&parentId=0&page=1&size=%d&enc=%s"

var reg_enc_fmt = regexp.MustCompile("enc[ ]*=\"(.*)\"")

func (driver ChaoxingDrive) Login(account *model.Account) error {
	url := "https://passport2.chaoxing.com/fanyalogin"
	var resp base.Json
	var err Resp

	req_body := fmt.Sprintf(form_login_fmt, account.Username, account.Password)

	loginReq, e := chaoxingClient.R().SetBody(req_body).
		SetResult(&resp).SetError(&err).
		SetHeader("Content-Length", strconv.FormatInt(int64(len(req_body)), 10)).
		SetHeader("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8").
		SetHeader("Host", "passport2.chaoxing.com").
		SetHeader("User-Agent", " Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/95.0.4638.69 Safari/537.36").
		SetHeader("X-Requested-With", "XMLHttpRequest").
		Post(url)

	if e != nil {
		return e
	}

	account.AccessToken = ""
	for _, cookie := range loginReq.Cookies() {
		//route=9d169c0aea4b7c89fa0d073417b5645f;
		account.AccessToken += fmt.Sprintf("%s=%s; ", cookie.Name, cookie.Value)
	}

	return nil
}

func (driver ChaoxingDrive) GetEnc(account *model.Account) error {
	url := "https://pan-yz.chaoxing.com/"

	encReq, e := chaoxingClient.R().SetHeader("Cookie", account.AccessToken).Get(url)
	if e != nil {
		return e
	}

	//??????????????????
	sresp := string(encReq.Body())
	submatch := reg_enc_fmt.FindAllStringSubmatch(sresp, 1)
	//??????????????????????????????????????????
	if len(submatch) == 0 {
		e = driver.Login(account)
		if e != nil {
			return e
		}
		encReq, e = chaoxingClient.R().SetHeader("Cookie", account.AccessToken).Get(url)
		if e != nil {
			return e
		}
		sresp = string(encReq.Body())
		submatch = reg_enc_fmt.FindAllStringSubmatch(sresp, 1)
		if len(submatch) == 0 {
			account.Status = "failed"
			return fmt.Errorf("???????????????????????????????????????%s", sresp)
		}
	}
	enc := submatch[0][1]
	account.AccessSecret = enc
	account.Status = "work"
	return nil
}

func (driver ChaoxingDrive) ListFile(folder_id string, account *model.Account) ([]model.File, error) {
	var url string
	//??????????????? id ???
	folder_id_info := strings.Split(folder_id, "_")
	if len(folder_id_info) == 3 {
		folder_id = folder_id_info[0]
		folder_puid := folder_id_info[1]
		folder_shareid := folder_id_info[2]
		if folder_id == "0" {
			//????????????????????????????????????
			url = fmt.Sprintf(api_list_shared_root, account.Limit, account.AccessSecret)
		} else {
			//??????????????????
			url = fmt.Sprintf(api_list_file, folder_puid, folder_shareid, folder_id, account.Limit, account.AccessSecret)
		}
	} else {
		//id???????????????????????????????????????????????????????????? ""???
		url = fmt.Sprintf(api_list_root, account.Limit, account.AccessSecret)
	}

	listFileReq, e := chaoxingClient.R().SetHeader("Cookie", account.AccessToken).Post(url)
	resp, e := simplejson.NewJson(listFileReq.Body())
	if e != nil || resp == nil {
		return nil, e
	}

	files := make([]model.File, 0)
	array, _ := resp.Get("list").Array()

	for _, file := range array {
		var f = model.File{}
		file_ := file.(map[string]interface{})
		//f.Id = file_["id"].(string)
		f.Id = fmt.Sprintf("%s_%s_%s", file_["id"].(string), file_["puid"].(json.Number).String(), file_["shareid"].(json.Number).String())
		f.Name = file_["name"].(string)
		f_server_type, _ := file_["type"].(json.Number).Int64()
		if f_server_type != TYPE_CX_SHARED_ROOT {
			f.Size, _ = file_["filesize"].(json.Number).Int64()
		}
		// ?????????????????????
		switch f_server_type {
		case TYPE_CX_FILE:
			{
				f.Type = utils.GetFileType(file_["suffix"].(string))
			}
		case TYPE_CX_FOLDER:
			f.Type = conf.FOLDER
		case TYPE_CX_SHARED_ROOT:
			f.Type = conf.FOLDER
		}
		modifyDate, e := time.Parse("2006-01-02 15:04:05", file_["modifyDate"].(string))
		if e == nil {
			f.UpdatedAt = &modifyDate
		}
		f.Thumbnail = file_["thumbnail"].(string)
		files = append(files, f)
	}
	return files, nil
}
