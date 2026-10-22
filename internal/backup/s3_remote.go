package backup

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

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
	if err := step("put", cl.Put(ctx, key, body), "Wrote "+s.Bucket+"/"+key+"."); err != nil {
		return steps, err
	}
	got, err := cl.Get(ctx, key)
	if err == nil && !bytes.Equal(got, body) {
		err = errors.New("the object read back is not the one written")
	}
	if err := step("get", err, "Read it back, byte for byte."); err != nil {
		_ = cl.Delete(context.WithoutCancel(ctx), key)
		return steps, err
	}
	if err := step("delete", cl.Delete(ctx, key), "Deleted it again."); err != nil {
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
