package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kelseyhightower/envconfig"
)

func main() {
	type ConfigSpecification struct {
		Debug    bool   `default:true`
		BaseURL  string `default:"https://demo.ui.com"`
		Username string `default:"admin"`
		Password string `default:"password"`
	}

	var config ConfigSpecification
	err := envconfig.Process("SwitchPortReset", &config)
	if err != nil {
		log.Fatal(err.Error())
	}

	type ResponseData struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}

	type ResetData struct {
		MacAddress string `json:"mac"`
		Port       int64  `json:"port_idx"`
		Command    string `json:"cmd"`
	}

	if !config.Debug {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.Default()

	// Setup

	router.GET("/reset/:searchMac", func(context *gin.Context) {
		searchMac := context.Param("searchMac")

		cookieJar, err := cookiejar.New(nil)
		if err != nil {
			context.AbortWithStatusJSON(http.StatusBadRequest, ResponseData{Success: false, Message: "Error preparing cookie jar: " + err.Error()})
		}

		// Login

		jsonLoginValue, err := json.Marshal(map[string]string{
			"username": config.Username,
			"password": config.Password,
		})
		if err != nil {
			context.AbortWithStatusJSON(http.StatusBadRequest, ResponseData{Success: false, Message: "Error logging in to UniFi server - processing JSON: " + err.Error()})
		}

		req, err := http.NewRequest("POST", config.BaseURL+"/api/login", bytes.NewBuffer(jsonLoginValue))
		if err != nil {
			context.AbortWithStatusJSON(http.StatusBadRequest, ResponseData{Success: false, Message: "Error logging in to UniFi server - preparing request: " + err.Error()})
		}
		req.Header.Set("Origin", config.BaseURL)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{
			Jar: cookieJar,
		}
		resp, err := client.Do(req)
		if err != nil {
			context.AbortWithStatusJSON(http.StatusBadRequest, ResponseData{Success: false, Message: "Error logging in to UniFi server: " + err.Error()})
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				context.AbortWithStatusJSON(http.StatusBadRequest, ResponseData{Success: false, Message: "Error logging in to UniFi server, returned status code: " + resp.Status + ". Error reading returned message: " + err.Error()})
			}
			bodyString := string(bodyBytes)
			context.AbortWithStatusJSON(http.StatusBadRequest, ResponseData{Success: false, Message: "Error logging in to UniFi server, returned status code: " + resp.Status + ". Returned message: " + bodyString})
		}

		// Get list of sites

		resp, err = client.Get(config.BaseURL + "/api/self/sites")

		if err != nil {
			context.AbortWithStatusJSON(http.StatusBadRequest, ResponseData{Success: false, Message: "Error getting list of available sites from UniFi server: " + err.Error()})
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				context.AbortWithStatusJSON(http.StatusBadRequest, ResponseData{Success: false, Message: "Error getting list of available sites from UniFi server, returned status code: " + resp.Status + ". Error reading returned message: " + err.Error()})
			}
			bodyString := string(bodyBytes)
			context.AbortWithStatusJSON(http.StatusBadRequest, ResponseData{Success: false, Message: "Error getting list of available sites from UniFi server, returned status code: " + resp.Status + ". Returned message: " + bodyString})
		}

		found := false

		var jsonData interface{}
		buf := new(bytes.Buffer)
		_, err = buf.ReadFrom(resp.Body)
		if err != nil {
			context.AbortWithStatusJSON(http.StatusBadRequest, ResponseData{Success: false, Message: "Error getting list of available sites from UniFi server - reading from response into buffer: " + err.Error()})
		}
		err = json.Unmarshal(buf.Bytes(), &jsonData)
		if err != nil {
			context.AbortWithStatusJSON(http.StatusBadRequest, ResponseData{Success: false, Message: "Error getting list of available sites from UniFi server - processing JSON: " + err.Error()})
		}

		sitesData := jsonData.(map[string]interface{})
		for _, siteJSON := range sitesData["data"].([]interface{}) {
			siteName := siteJSON.(map[string]interface{})["name"].(string)

			// Get clients

			resp, err := client.Get(config.BaseURL + "/api/s/" + siteName + "/stat/sta")

			if err != nil {
				context.AbortWithStatusJSON(http.StatusBadRequest, ResponseData{Success: false, Message: "Error getting list of clients from UniFi server: " + err.Error()})
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				bodyBytes, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					context.AbortWithStatusJSON(http.StatusBadRequest, ResponseData{Success: false, Message: "Error getting list of clients from UniFi server, returned status code: " + resp.Status + ". Error reading returned message: " + err.Error()})
				}
				bodyString := string(bodyBytes)
				context.AbortWithStatusJSON(http.StatusBadRequest, ResponseData{Success: false, Message: "Error getting list of clients from UniFi server, returned status code: " + resp.Status + ". Returned message: " + bodyString})
			}

			buf = new(bytes.Buffer)
			_, err = buf.ReadFrom(resp.Body)
			if err != nil {
				context.AbortWithStatusJSON(http.StatusBadRequest, ResponseData{Success: false, Message: "Error getting list of clients from UniFi server - reading from response into buffer: " + err.Error()})
			}
			err = json.Unmarshal(buf.Bytes(), &jsonData)
			if err != nil {
				context.AbortWithStatusJSON(http.StatusBadRequest, ResponseData{Success: false, Message: "Error getting list of clients from UniFi server - processing JSON: " + err.Error()})
			}
			clientsData := jsonData.(map[string]interface{})
			for _, clientJSON := range clientsData["data"].([]interface{}) {

				// Find client

				macAddress := clientJSON.(map[string]interface{})["mac"].(string)
				if strings.ToLower(macAddress) == strings.ToLower(searchMac) && clientJSON.(map[string]interface{})["is_wired"].(bool) {
					found = true
					switchMac := clientJSON.(map[string]interface{})["sw_mac"].(string)
					switchPort := clientJSON.(map[string]interface{})["sw_port"].(float64)

					// Reset port power

					resetValues := ResetData{
						MacAddress: switchMac,
						Port:       int64(switchPort),
						Command:    "power-cycle",
					}

					jsonResetValue, err := json.Marshal(resetValues)
					if err != nil {
						context.AbortWithStatusJSON(http.StatusBadRequest, ResponseData{Success: false, Message: "Error requesting reset of switch port - processing JSON: " + err.Error()})
					}

					req, err = http.NewRequest("POST", config.BaseURL+"/api/s/"+siteName+"/cmd/devmgr", bytes.NewBuffer(jsonResetValue))
					if err != nil {
						context.AbortWithStatusJSON(http.StatusBadRequest, ResponseData{Success: false, Message: "Error requesting reset of switch port - preparing request: " + err.Error()})
					}
					req.Header.Set("Origin", config.BaseURL)
					req.Header.Set("Content-Type", "application/json")

					resp, err = client.Do(req)
					if err != nil {
						context.AbortWithStatusJSON(http.StatusBadRequest, ResponseData{Success: false, Message: "Error requesting reset of switch port: " + err.Error()})
					}
					defer resp.Body.Close()

					if resp.StatusCode != http.StatusOK {
						bodyBytes, err := ioutil.ReadAll(resp.Body)
						if err != nil {
							context.AbortWithStatusJSON(http.StatusBadRequest, ResponseData{Success: false, Message: "Error requesting reset of switch port, returned status code: " + resp.Status + ". Error reading returned message: " + err.Error()})
						}
						bodyString := string(bodyBytes)
						context.AbortWithStatusJSON(http.StatusBadRequest, ResponseData{Success: false, Message: "Error requesting reset of switch port, returned status code: " + resp.Status + ". Returned message: " + bodyString})
					}

					context.JSON(http.StatusOK, ResponseData{Success: true, Message: "Successfully reset power to switch port connected to mac address " + strings.ToLower(searchMac)})

				}
			}

		}
		if !found {
			context.AbortWithStatusJSON(http.StatusBadRequest, ResponseData{Success: false, Message: "No devices found on UniFi server with mac address " + strings.ToLower(searchMac)})
		}
	})
	router.Run(":9000")

}
