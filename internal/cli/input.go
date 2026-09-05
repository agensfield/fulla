package cli

import (
	"crypto/rand"
	"io"
	"math/big"
	"os"
	"strconv"

	"github.com/agensfield/fulla/internal/config"
	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
)

func (a *App) input(p invocation, c *config.Resolved) ([]byte, error) {
	sources := 0
	for _, flag := range []string{"stdin", "from-fd", "generate"} {
		if p.has(flag) {
			sources++
		}
	}
	if sources > 1 {
		return nil, fault.Usage("select exactly one input source")
	}
	if sources == 0 {
		return nil, fault.Interaction("provide --stdin, --from-fd N, or --generate")
	}
	if (p.has("length") || p.has("alphabet")) && !p.has("generate") {
		return nil, fault.Usage("generation settings require --generate")
	}
	if p.has("generate") {
		length := c.Generation.Length
		alphabet := c.Generation.Alphabet
		if p.has("length") {
			n, err := strconv.Atoi(p.value("length"))
			if err != nil || n < 1 || n > 65536 {
				return nil, fault.Usage("length must be between 1 and 65536")
			}
			length = n
		}
		if p.has("alphabet") {
			alphabet = p.value("alphabet")
		}
		if len(alphabet) < 2 || len(alphabet) > 256 {
			return nil, fault.Usage("alphabet needs 2 to 256 distinct printable ASCII characters")
		}
		seen := map[byte]bool{}
		for _, b := range []byte(alphabet) {
			if b < 33 || b > 126 || seen[b] {
				return nil, fault.Usage("alphabet must contain distinct printable ASCII characters")
			}
			seen[b] = true
		}
		value := make([]byte, length)
		for i := range value {
			n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
			if err != nil {
				return nil, err
			}
			value[i] = alphabet[n.Int64()]
		}
		return value, nil
	}
	reader := a.In
	if p.has("from-fd") {
		fd, err := strconv.Atoi(p.value("from-fd"))
		if err != nil || fd < 0 || fd == 1 || fd == 2 {
			return nil, fault.Usage("from-fd requires an inherited readable descriptor other than stdout/stderr")
		}
		f := os.NewFile(uintptr(fd), "fulla-input")
		if f == nil {
			return nil, fault.Usage("invalid input descriptor")
		}
		defer f.Close()
		reader = f
	}
	value, err := io.ReadAll(io.LimitReader(reader, crypt.MaxEntryBytes+1))
	if err != nil {
		return nil, fault.New("input.failed", "could not read selected input")
	}
	if len(value) > crypt.MaxEntryBytes {
		return nil, fault.New("entry.too_large", "entry exceeds the supported size limit")
	}
	return value, nil
}
