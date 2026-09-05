package crypt

import (
	"io"

	"filippo.io/age"
	"github.com/agensfield/fulla/internal/fault"
)

// EncryptStream applies the same plugin policy as entry encryption. Callers
// retain responsibility for payload bounds, closing the writer, and publication.
func EncryptStream(output io.Writer, recipients []age.Recipient) (io.WriteCloser, error) {
	if err := rejectPluginDebug(nil, recipients); err != nil {
		return nil, err
	}
	writer, err := age.Encrypt(output, recipients...)
	if err != nil {
		if failure := pluginFailure(err); failure != nil {
			return nil, failure
		}
		return nil, fault.New("crypto.encrypt_failed", "could not wrap encrypted file key")
	}
	return writer, nil
}

// DecryptStream authenticates the header and applies plugin policy. Callers must
// consume the bounded payload through authenticated EOF before publishing it.
func DecryptStream(input io.Reader, identities []age.Identity) (io.Reader, error) {
	if err := rejectPluginDebug(identities, nil); err != nil {
		return nil, err
	}
	reader, err := age.Decrypt(input, identities...)
	if err != nil {
		if failure := pluginFailure(err); failure != nil {
			return nil, failure
		}
		return nil, fault.New("crypto.decrypt_failed", "could not decrypt with the selected identity")
	}
	return reader, nil
}
