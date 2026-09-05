package store

import (
	"strings"

	"github.com/agensfield/fulla/internal/fault"
)

// checkGitConversions asks Git to resolve attribute precedence without running
// conversion commands. Internal history must retain the exact ciphertext bytes.
// Include pa's root metadata file because status may refresh its index entry too.
func (s *Store) checkGitConversions(names []string) error {
	enabled, err := s.GitEnabled()
	if err != nil || !enabled {
		return err
	}
	paths := []string{".gitattributes"}
	for _, name := range names {
		if _, err := EntryPath(name); err != nil {
			return err
		}
		paths = append(paths, name+".age")
	}
	for len(paths) > 0 {
		// Entry names may be 4 KiB; bound argv below macOS command limits.
		count := min(len(paths), 32)
		args := append([]string{"check-attr", "-z", "--all", "--"}, paths[:count]...)
		out, err := s.Git(args...)
		if err != nil {
			return err
		}
		fields := strings.Split(string(out), "\x00")
		if fields[len(fields)-1] != "" || (len(fields)-1)%3 != 0 {
			return fault.New("git.attributes_invalid", "could not validate Git conversion attributes")
		}
		for i := 0; i+2 < len(fields); i += 3 {
			attribute, value := fields[i+1], fields[i+2]
			unsafe := false
			switch attribute {
			case "filter", "working-tree-encoding":
				// --all omits unspecified attributes. Even an explicit unset is
				// conservatively refused: Git's output cannot distinguish it
				// from a literal driver/encoding named "unset".
				unsafe = true
			case "text", "crlf", "eol", "ident":
				unsafe = value != "unset"
			}
			if unsafe {
				return fault.New("git.conversion_unsupported", "Git conversion attributes are unsupported for ciphertext and repository metadata; remove the conversion rule before retrying")
			}
		}
		paths = paths[count:]
	}
	return nil
}
