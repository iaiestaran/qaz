package commands

import (
	"fmt"
	"io/ioutil"
	"strings"
	"text/template"

	"github.com/daidokoro/qaz/bucket"
	stks "github.com/daidokoro/qaz/stacks"
	"github.com/daidokoro/qaz/utils"

	"encoding/base64"
	"encoding/json"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/kms"
)

// Common Functions - Both Deploy/Gen
var (
	kmsEncrypt = func(kid string, text string) string {
		log.Debug("running template function: [kms_encrypt]")
		sess, err := GetSession()
		utils.HandleError(err)

		svc := kms.New(sess)

		params := &kms.EncryptInput{
			KeyId:     aws.String(kid),
			Plaintext: []byte(text),
		}

		resp, err := svc.Encrypt(params)
		utils.HandleError(err)

		return base64.StdEncoding.EncodeToString(resp.CiphertextBlob)
	}

	kmsDecrypt = func(cipher string) string {
		log.Debug("running template function: [kms_decrypt]")
		sess, err := GetSession()
		utils.HandleError(err)

		svc := kms.New(sess)

		ciph, err := base64.StdEncoding.DecodeString(cipher)
		utils.HandleError(err)

		params := &kms.DecryptInput{
			CiphertextBlob: []byte(ciph),
		}

		resp, err := svc.Decrypt(params)
		utils.HandleError(err)

		return string(resp.Plaintext)
	}

	httpGet = func(url string) interface{} {
		log.Debug("Calling Template Function [GET] with arguments: %s", url)
		resp, err := utils.Get(url)
		utils.HandleError(err)

		return resp
	}

	s3Read = func(url string, profile ...string) string {
		log.Debug("Calling Template Function [S3Read] with arguments: %s", url)

		sess, err := GetSession(func(opts *session.Options) {
			if len(profile) < 1 {
				log.Warn("No Profile specified for S3read, using: %s", run.profile)
				return
			}
			opts.Profile = profile[0]
			return
		})

		utils.HandleError(err)

		resp, err := bucket.S3Read(url, sess)
		utils.HandleError(err)

		return resp
	}

	lambdaInvoke = func(name string, payload string) interface{} {
		log.Debug("running template function: [invoke]")
		f := awsLambda{name: name}
		var m interface{}

		if payload != "" {
			f.payload = []byte(payload)
		}

		sess, err := GetSession()
		utils.HandleError(err)

		err = f.Invoke(sess)
		utils.HandleError(err)

		log.Debug("Lambda response: %s", f.response)

		// parse json if possible
		if err := json.Unmarshal([]byte(f.response), &m); err != nil {
			log.Debug(err.Error())
			return f.response
		}

		return m
	}

	prefix = func(s string, pre string) bool {
		return strings.HasPrefix(s, pre)
	}

	suffix = func(s string, suf string) bool {
		return strings.HasSuffix(s, suf)
	}

	contains = func(s string, con string) bool {
		return strings.Contains(s, con)
	}

	loop = func(n int) []struct{} {
		return make([]struct{}, n)
	}

	literal = func(str string) string {
		return fmt.Sprintf("%#v", str)
	}

	// gentime function maps
	GenTimeFunctions = template.FuncMap{
		// simple additon function useful for counters in loops
		"add": func(a int, b int) int {
			log.Debug("Calling Template Function [add] with arguments: %s + %s", a, b)
			return a + b
		},

		// strip function for removing characters from text
		"strip": func(s string, rmv string) string {
			log.Debug("Calling Template Function [strip] with arguments: (%s, %s) ", s, rmv)
			return strings.Replace(s, rmv, "", -1)
		},

		// cat function for reading text from a given file under the files folder
		"cat": func(path string) string {
			log.Debug("Calling Template Function [cat] with arguments: %s", path)
			b, err := ioutil.ReadFile(path)
			utils.HandleError(err)
			return string(b)
		},

		// literal - return raw/literal string with special chars and all
		"literal": literal,

		// suffix - returns true if string starts with given suffix
		"suffix": suffix,

		// prefix - returns true if string starts with given prefix
		"prefix": prefix,

		// contains - returns true if string contains
		"contains": contains,

		// loop - useful to range over an int (rather than a slice, map, or channel). see examples/loop
		"loop": loop,

		// Get get does an HTTP Get request of the given url and returns the output string
		"GET": httpGet,

		// S3Read reads content of file from s3 and returns string contents
		"s3_read": s3Read,

		// invoke - invokes a lambda function and returns a raw string/interface{}
		"invoke": lambdaInvoke,

		// kms-encrypt - Encrypts PlainText using KMS key
		"kms_encrypt": kmsEncrypt,

		// kms-decrypt - Descrypts CipherText
		"kms_decrypt": kmsDecrypt,

		// mod - returns remainder after division (modulus)
		"mod": func(a int, b int) int {
			log.Debug("Calling Template Function [mod] with arguments: %s % %s", a, b)
			return a % b
		},
	}

	// deploytime function maps
	DeployTimeFunctions = template.FuncMap{
		// Fetching stackoutputs
		"stack_output": func(target string) string {
			log.Debug("Deploy-Time function resolving: %s", target)
			req := strings.Split(target, "::")

			s, ok := stacks.Get(req[0])
			if !ok {
				utils.HandleError(fmt.Errorf("stack_output errror: stack [%s] not found", req[0]))
			}

			utils.HandleError(s.Outputs())

			for _, i := range s.Output.Stacks {
				for _, o := range i.Outputs {
					if *o.OutputKey == req[1] {
						return *o.OutputValue
					}
				}
			}
			utils.HandleError(fmt.Errorf("Stack Output Not found - Stack:%s | Output:%s", req[0], req[1]))
			return ""
		},

		"stack_output_ext": func(target string) string {
			log.Debug("Deploy-Time function resolving: %s", target)
			req := strings.Split(target, "::")

			sess, err := GetSession()
			utils.HandleError(err)

			s := stks.Stack{
				Stackname: req[0],
				Session:   sess,
			}

			err = s.Outputs()
			utils.HandleError(err)

			for _, i := range s.Output.Stacks {
				for _, o := range i.Outputs {
					if *o.OutputKey == req[1] {
						return *o.OutputValue
					}
				}
			}

			utils.HandleError(fmt.Errorf("Stack Output Not found - Stack:%s | Output:%s", req[0], req[1]))
			return ""
		},

		// suffix - returns true if string starts with given suffix
		"suffix": suffix,

		// prefix - returns true if string starts with given prefix
		"prefix": prefix,

		// contains - returns true if string contains
		"contains": contains,

		// loop - useful to range over an int (rather than a slice, map, or channel). see examples/loop
		"loop": loop,

		// literal - return raw/literal string with special chars and all
		"literal": literal,

		// Get get does an HTTP Get request of the given url and returns the output string
		"GET": httpGet,

		// S3Read reads content of file from s3 and returns string contents
		"s3_read": s3Read,

		// invoke - invokes a lambda function and returns a raw string/interface{}
		"invoke": lambdaInvoke,

		// kms-encrypt - Encrypts PlainText using KMS key
		"kms_encrypt": kmsEncrypt,

		// kms-decrypt - Descrypts CipherText
		"kms_decrypt": kmsDecrypt,
	}
)
