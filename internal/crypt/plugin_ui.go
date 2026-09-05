package crypt

import (
	"errors"
	"sync"

	"filippo.io/age"
	"filippo.io/age/plugin"
	"github.com/agensfield/fulla/internal/ageplugin"
	"github.com/agensfield/fulla/internal/fault"
)

// age requires a non-nil ClientUI and converts callback errors into protocol
// failures. Keep a per-operation error so input refusal survives that boundary.
// Serialize reuse of a parsed plugin object; no state is shared across objects.
type pluginInteraction struct {
	mu      sync.Mutex
	failure error
}

func pluginUI(source *plugin.ClientUI) (*ageplugin.ClientUI, *pluginInteraction) {
	state := &pluginInteraction{}
	if source == nil {
		source = &plugin.ClientUI{}
	}
	ui := &ageplugin.ClientUI{WaitTimer: source.WaitTimer}
	refuse := func() error {
		state.failure = fault.Interaction("age plugin requires interactive input")
		return state.failure
	}
	failed := func(err error) error {
		if err != nil {
			state.failure = fault.New("plugin.interaction_failed", "age plugin interaction did not complete")
			var problem *fault.Error
			if errors.As(err, &problem) {
				switch problem.Code {
				case "interaction.required":
					state.failure = fault.Interaction("age plugin requires interactive input")
				case "input.cancelled", "plugin.interaction_cancelled":
					cancelled := fault.New("plugin.interaction_cancelled", "age plugin interaction cancelled")
					switch problem.Status {
					case 129, 130, 131, 143:
						cancelled.Status = problem.Status
					}
					state.failure = cancelled
				}
			}
		}
		return err
	}
	ui.DisplayMessage = func(name, message string) error {
		if source.DisplayMessage == nil {
			return nil // Informational messages never become unsolicited output.
		}
		return failed(source.DisplayMessage(name, message))
	}
	ui.RequestValue = func(name, prompt string, secret bool) (string, error) {
		if source.RequestValue == nil {
			return "", refuse()
		}
		value, err := source.RequestValue(name, prompt, secret)
		return value, failed(err)
	}
	ui.Confirm = func(name, prompt, yes, no string) (bool, error) {
		if source.Confirm == nil {
			return false, refuse()
		}
		value, err := source.Confirm(name, prompt, yes, no)
		return value, failed(err)
	}
	return ui, state
}

type pluginRecipient struct {
	*ageplugin.Recipient
	interaction *pluginInteraction
}

func (r *pluginRecipient) Wrap(key []byte) ([]*age.Stanza, error) {
	stanzas, _, err := r.WrapWithLabels(key)
	return stanzas, err
}

func (r *pluginRecipient) WrapWithLabels(key []byte) ([]*age.Stanza, []string, error) {
	r.interaction.mu.Lock()
	defer r.interaction.mu.Unlock()
	r.interaction.failure = nil
	stanzas, labels, err := r.Recipient.WrapWithLabels(key)
	if r.interaction.failure != nil {
		return nil, nil, r.interaction.failure
	}
	return stanzas, labels, err
}

type pluginIdentity struct {
	*ageplugin.Identity
	interaction *pluginInteraction
}

func (i *pluginIdentity) Unwrap(stanzas []*age.Stanza) ([]byte, error) {
	i.interaction.mu.Lock()
	defer i.interaction.mu.Unlock()
	i.interaction.failure = nil
	key, err := i.Identity.Unwrap(stanzas)
	if i.interaction.failure != nil {
		clear(key)
		return nil, i.interaction.failure
	}
	return key, err
}

func pluginName(value any) (string, bool) {
	switch value.(type) {
	case *plugin.Identity, *plugin.Recipient, *ageplugin.Identity, *ageplugin.Recipient, *pluginIdentity, *pluginRecipient:
		return value.(interface{ Name() string }).Name(), true
	}
	return "", false
}
