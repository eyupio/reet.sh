package backup

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The S3 client is deliberately four verbs and a signature: put, get, list
// and delete, signed with Signature Version 4. Anything that implements the
// S3 API speaks it -- AWS, MinIO, Cloudflare R2, Backblaze B2, Garage -- and it
// costs less to read than a dependency tree would.

const (
	emptyPayloadSHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	defaultS3Region    = "us-east-1"
	maxObjectBytes     = 512 << 20
)

type s3Client struct {
	endpoint  *url.URL
	bucket    string
	region    string
	accessKey string
	secretKey string
	pathStyle bool
	http      *http.Client
	now       func() time.Time
}

var s3Transport = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   15 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	TLSHandshakeTimeout:   15 * time.Second,
	ResponseHeaderTimeout: 60 * time.Second,
	MaxIdleConnsPerHost:   2,
}

func newS3Client(c S3) (*s3Client, error) {
	u, err := url.Parse(strings.TrimSpace(c.Endpoint))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("%q is not an S3 endpoint URL; write one like https://s3.eu-west-2.amazonaws.com or http://minio:9000", c.Endpoint)
	}
	if strings.TrimSpace(c.Bucket) == "" {
		return nil, errors.New("the destination names no bucket")
	}
	if strings.TrimSpace(c.AccessKey) == "" || strings.TrimSpace(c.SecretKey) == "" {
		return nil, errors.New("the destination has no access key id and secret access key")
	}
	region := strings.TrimSpace(c.Region)
	if region == "" {
		region = defaultS3Region
	}
	style := !isAWSEndpoint(u.Hostname())
	if c.PathStyle != nil {
		style = *c.PathStyle
	}
	return &s3Client{
		endpoint: u, bucket: strings.TrimSpace(c.Bucket), region: region,
		accessKey: strings.TrimSpace(c.AccessKey), secretKey: strings.TrimSpace(c.SecretKey),
		pathStyle: style, http: &http.Client{Transport: s3Transport}, now: time.Now,
	}, nil
}

func isAWSEndpoint(host string) bool {
	host = strings.ToLower(host)
	return host == "amazonaws.com" || strings.HasSuffix(host, ".amazonaws.com")
}

func (c *s3Client) url(key string) *url.URL {
	u := *c.endpoint
	if c.pathStyle {
		u.Path = "/" + c.bucket
		if key != "" {
			u.Path += "/" + key
		}
	} else {
		u.Host = c.bucket + "." + u.Host
		u.Path = "/" + key
	}
	u.RawPath = canonicalPath(&u)
	return &u
}

type s3Object struct {
	Key          string
	Size         int64
	LastModified time.Time
}

type listResult struct {
	XMLName               xml.Name `xml:"ListBucketResult"`
	IsTruncated           bool     `xml:"IsTruncated"`
	NextContinuationToken string   `xml:"NextContinuationToken"`
	Contents              []struct {
		Key          string    `xml:"Key"`
		Size         int64     `xml:"Size"`
		LastModified time.Time `xml:"LastModified"`
	} `xml:"Contents"`
}

func (c *s3Client) list(ctx context.Context, prefix string) ([]s3Object, error) {
	var out []s3Object
	token := ""
	for {
		q := url.Values{"list-type": {"2"}}
		if prefix != "" {
			q.Set("prefix", prefix)
		}
		if token != "" {
			q.Set("continuation-token", token)
		}
		u := c.url("")
		u.RawQuery = q.Encode()
		// Send the query exactly as it is signed: q.Encode writes a space as
		// "+", which the signature counts as a different byte.
		u.RawQuery = canonicalQuery(u)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.do(req, emptyPayloadSHA256)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading the listing of %s: %w", c.bucket, err)
		}
		var parsed listResult
		if err := xml.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("%s answered a listing this is not: %w", c.endpoint.Host, err)
		}
		for _, item := range parsed.Contents {
			out = append(out, s3Object{Key: item.Key, Size: item.Size, LastModified: item.LastModified})
		}
		if !parsed.IsTruncated || parsed.NextContinuationToken == "" {
			return out, nil
		}
		token = parsed.NextContinuationToken
	}
}

// putRetry sends a copy up to three times when the store may pass: a 5xx,
// 429 or 408, or a connection that failed or timed out. A refusal that will
// not pass is attempted once.
func (c *s3Client) putRetry(ctx context.Context, key string, body []byte) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = c.put(ctx, key, body)
		if err == nil || !retryableS3(err) {
			return err
		}
		wait := time.Duration(attempt+1) * 500 * time.Millisecond
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	return err
}

func retryableS3(err error) bool {
	msg := strings.ToLower(err.Error())
	for _, s := range []string{
		"answered 5", "answered 429", "answered 408", "slowdown", "internalerror",
		"requesttimeout", "timeout", "connection reset", "connection refused",
		"no such host", "unexpected eof", "broken pipe", "use of closed network connection",
	} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

func (c *s3Client) put(ctx context.Context, key string, body []byte) error {
	sum := sha256.Sum256(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.url(key).String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := c.do(req, hex.EncodeToString(sum[:]))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return nil
}

func (c *s3Client) get(ctx context.Context, key string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(key).String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req, emptyPayloadSHA256)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.ContentLength > maxObjectBytes {
		return nil, fmt.Errorf("%s is %d bytes, larger than this build can read from a destination (%d); restore it from the copy on this machine instead", key, resp.ContentLength, int64(maxObjectBytes))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxObjectBytes))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", key, err)
	}
	return body, nil
}

func (c *s3Client) remove(ctx context.Context, key string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.url(key).String(), nil)
	if err != nil {
		return err
	}
	resp, err := c.do(req, emptyPayloadSHA256)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return nil
}

func (c *s3Client) do(req *http.Request, payloadSHA256 string) (*http.Response, error) {
	if err := c.sign(req, payloadSHA256); err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", req.Method, redactedURL(req.URL), err)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
	return nil, s3Error(req, resp.StatusCode, body)
}

type s3ErrorBody struct {
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

func s3Error(req *http.Request, status int, body []byte) error {
	var parsed s3ErrorBody
	_ = xml.Unmarshal(body, &parsed)
	where := redactedURL(req.URL)
	switch parsed.Code {
	case "NoSuchBucket":
		return fmt.Errorf("the bucket does not exist at %s", where)
	case "AccessDenied":
		return fmt.Errorf("%s refused the request as %s; the credentials need PutObject, GetObject, DeleteObject and ListBucket on the bucket and its prefix", where, req.Method)
	case "InvalidAccessKeyId":
		return fmt.Errorf("%s does not know this access key id", where)
	case "SignatureDoesNotMatch":
		return fmt.Errorf("%s rejected the signature; the secret access key or the region is wrong", where)
	case "RequestTimeTooSkewed":
		return fmt.Errorf("%s refused the request because this host's clock is too far from its own", where)
	case "NoSuchKey":
		return fmt.Errorf("%s holds no such object", where)
	case "PermanentRedirect", "AuthorizationHeaderMalformed":
		return fmt.Errorf("%s says this bucket belongs to another region (%s)", where, strings.TrimSpace(parsed.Message))
	}
	switch {
	case status == http.StatusNotFound:
		return fmt.Errorf("%s answered 404; the bucket or the object is not there", where)
	case status == http.StatusForbidden:
		return fmt.Errorf("%s answered 403; the credentials were refused or the policy does not allow %s", where, req.Method)
	case parsed.Message != "":
		return fmt.Errorf("%s answered %d: %s (%s)", where, status, strings.TrimSpace(parsed.Message), parsed.Code)
	default:
		return fmt.Errorf("%s answered %d", where, status)
	}
}

func redactedURL(u *url.URL) string {
	return u.Scheme + "://" + u.Host + u.EscapedPath()
}

func (c *s3Client) sign(req *http.Request, payloadSHA256 string) error {
	now := c.now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	req.Header.Set("Host", req.URL.Host)
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadSHA256)
	signed, canonicalHeaders := canonicalHeaders(req)
	canonicalRequest := strings.Join([]string{
		req.Method, canonicalPath(req.URL), canonicalQuery(req.URL), canonicalHeaders, signed, payloadSHA256,
	}, "\n")
	scope := strings.Join([]string{dateStamp, c.region, "s3", "aws4_request"}, "/")
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256", amzDate, scope, hex.EncodeToString(sha256Of(canonicalRequest)),
	}, "\n")
	key := hmacSHA256([]byte("AWS4"+c.secretKey), dateStamp)
	key = hmacSHA256(key, c.region)
	key = hmacSHA256(key, "s3")
	key = hmacSHA256(key, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(key, stringToSign))
	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		c.accessKey, scope, signed, signature))
	return nil
}

func canonicalHeaders(req *http.Request) (signed, block string) {
	names := []string{"host", "x-amz-content-sha256", "x-amz-date"}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		value := req.Header.Get(name)
		if name == "host" {
			value = req.URL.Host
		}
		b.WriteString(name)
		b.WriteString(":")
		b.WriteString(strings.TrimSpace(value))
		b.WriteString("\n")
	}
	return strings.Join(names, ";"), b.String()
}

func canonicalPath(u *url.URL) string {
	path := u.Path
	if path == "" {
		return "/"
	}
	parts := strings.Split(path, "/")
	for i, p := range parts {
		parts[i] = uriEncode(p, false)
	}
	return strings.Join(parts, "/")
}

func canonicalQuery(u *url.URL) string {
	values := u.Query()
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	var pairs []string
	for _, name := range names {
		vs := values[name]
		sort.Strings(vs)
		for _, v := range vs {
			pairs = append(pairs, uriEncode(name, true)+"="+uriEncode(v, true))
		}
	}
	return strings.Join(pairs, "&")
}

func uriEncode(s string, encodeSlash bool) string {
	var b strings.Builder
	for _, r := range []byte(s) {
		switch {
		case (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'),
			r == '-', r == '_', r == '.', r == '~':
			b.WriteByte(r)
		case r == '/' && !encodeSlash:
			b.WriteByte(r)
		default:
			fmt.Fprintf(&b, "%%%02X", r)
		}
	}
	return b.String()
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func sha256Of(s string) []byte {
	sum := sha256.Sum256([]byte(s))
	return sum[:]
}

// CheckStep is one request of a destination test.
type CheckStep struct {
	Step   string `json:"step"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// Test writes a small object under the prefix, reads it back and deletes it:
// the three things a backup needs of a store, in the order it needs them.
func Test(ctx context.Context, s S3) ([]CheckStep, error) {
	cl, err := newS3Client(s)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	nonce, _ := randomHex(8)
	key := s.Prefix + ".connection-test/" + nonce + ".txt"
	body := []byte("A connection test from reet.sh\n")
	var steps []CheckStep
	step := func(name string, err error, ok string) error {
		if err != nil {
			steps = append(steps, CheckStep{Step: name, Detail: err.Error()})
			return err
		}
		steps = append(steps, CheckStep{Step: name, OK: true, Detail: ok})
		return nil
	}
	if err := step("put", cl.put(ctx, key, body), "Wrote "+s.Bucket+"/"+key+"."); err != nil {
		return steps, err
	}
	got, err := cl.get(ctx, key)
	if err == nil && !bytes.Equal(got, body) {
		err = errors.New("the object read back is not the one written")
	}
	if err := step("get", err, "Read it back, byte for byte."); err != nil {
		_ = cl.remove(context.WithoutCancel(ctx), key)
		return steps, err
	}
	if err := step("delete", cl.remove(ctx, key), "Deleted it again."); err != nil {
		return steps, err
	}
	return steps, nil
}

func remotePath(prefix, scope string, ownerID int64, id string) string {
	prefix = strings.Trim(prefix, "/")
	if prefix != "" {
		prefix += "/"
	}
	if scope == "instance" {
		return prefix + "instance/" + id + ".reetbackup"
	}
	return prefix + "user-" + strconv.FormatInt(ownerID, 10) + "/" + id + ".reetbackup"
}
