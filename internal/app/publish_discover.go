package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gitmake/internal/config"
)

// runPublish carried its five pipeline stages inline. The stages were already
// named -- the function announces DISCOVER, PLAN, PREPARE, SECURITY and
// VALIDATE as it goes -- but they shared one 295-line scope, so none of them
// could be read or tested on its own. This file lifts the first of them out.
//
// Only the boundary moves. What DISCOVER does, in what order, and what it
// writes into the pipeline state is unchanged: that output is a machine
// contract.

// publishInput is everything DISCOVER resolves: which source is being
// published, under which configuration, and the hashes that later stages bind
// a reviewed plan to.
type publishInput struct {
	CWD        string
	ConfigPath string
	Config     config.Config
	Source     sourceSelection

	// ConfigSource records where the configuration came from: file, stdin,
	// inferred, or project_memory. It reaches the user and the JSON output.
	ConfigSource    string
	ConfigPersisted bool

	SourceSHA256 string
	ConfigSHA256 string

	// MemoryUsed reports that the repository target came from folder project
	// memory, which lets a later stage adopt the remote's visibility rather
	// than the inferred default.
	MemoryUsed bool
}

// errSourceGuidanceShown reports that GitMake could not select a source, has
// already explained that to the user in full, and the command should end
// successfully without publishing. It exists so the guidance path can leave
// discoverPublishInput without pretending to be a failure.
var errSourceGuidanceShown = errors.New("source guidance already shown")

// discoverPublishInput resolves the source and configuration for a publish.
//
// It writes the discovery, source and config portions of the pipeline state
// and, on success, advances that state to PLAN -- exactly where the inline
// code did.
func discoverPublishInput(o Options) (publishInput, error) {
	var in publishInput
	if o.State != nil {
		o.State.enter("DISCOVER")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return in, err
	}
	in.CWD = cwd
	in.ConfigPath = resolveConfigPath(cwd, o.ConfigPath)
	in.ConfigSource = "file"

	// A positional path is the strongest source-selection signal. v0.8 accepts
	// either a project folder or a ZIP snapshot.
	explicit, err := explicitSource(cwd, o.SourceArg)
	if err != nil {
		return in, err
	}

	configExists := false
	if _, statErr := os.Stat(in.ConfigPath); statErr == nil {
		configExists = true
	} else if !os.IsNotExist(statErr) {
		return in, fmt.Errorf("check config: %w", statErr)
	}

	switch {
	case o.Stdin:
		incoming, readErr := readLimitedStdin()
		if readErr != nil {
			return in, readErr
		}
		in.Config, err = config.ParseBytes(incoming)
		if err != nil {
			return in, err
		}
		in.ConfigSource = "stdin"
		in.Source = explicit
		if in.Source.Path == "" {
			in.Source, err = configuredSource(in.ConfigPath, &in.Config, true)
			if err != nil {
				return in, err
			}
		}

	case configExists:
		in.Config, err = config.Load(in.ConfigPath)
		if err != nil {
			return in, err
		}
		in.ConfigPersisted = true
		in.Source = explicit
		if in.Source.Path == "" {
			in.Source, err = configuredSource(in.ConfigPath, &in.Config, o.ReadOnly)
			if err != nil {
				return in, err
			}
		}

	default:
		in.ConfigSource = "inferred"
		in.Source = explicit
		if in.Source.Path == "" {
			in.Source, err = inferSource(cwd)
			if in.Source.Discovery != nil && o.State != nil {
				o.State.Discovery = discoveryStateFromReport(*in.Source.Discovery)
			}
			if err != nil {
				if guideErr := explainUnresolvedSource(o, in.Source, err); guideErr != nil {
					return in, guideErr
				}
			}
		}

		in.Config, err = configForSelection(in.ConfigPath, in.Source)
		if err != nil {
			return in, err
		}
		// v0.9 is zero-config by default. Missing gitmake.json stays in
		// memory unless the user explicitly runs `gitmake init` or a config
		// authoring command. This keeps the simple path free of setup files.
	}

	in.Source.Path, err = filepath.Abs(in.Source.Path)
	if err != nil {
		return in, fmt.Errorf("resolve source path: %w", err)
	}

	// Explicit stdin configuration is authoritative. Project memory may still
	// block an unsafe retarget later, but it must never silently rewrite the
	// caller's repo/branch/visibility choices.
	if !in.ConfigPersisted && in.ConfigSource != "stdin" {
		in.MemoryUsed, err = applyFolderProjectMemory(in.Source, &in.Config)
		if err != nil {
			return in, err
		}
		if in.MemoryUsed {
			in.ConfigSource = "project_memory"
		}
	}

	in.SourceSHA256, err = hashSelectedSource(in.Source)
	if err != nil {
		return in, fmt.Errorf("hash source %s: %w", in.Source.Mode, err)
	}
	if in.ConfigPersisted {
		in.ConfigSHA256, err = sha256File(in.ConfigPath)
		if err != nil {
			return in, fmt.Errorf("hash config: %w", err)
		}
	} else if in.ConfigSource == "stdin" {
		normalized, normErr := config.MarshalNormalized(in.Config)
		if normErr != nil {
			return in, normErr
		}
		in.ConfigSHA256 = sha256Bytes(normalized)
	}

	if o.State != nil {
		o.State.SourceMode = in.Source.Mode
		o.State.Source = sourceDisplay(in.Source)
		o.State.SourcePath = in.Source.Path
		o.State.SourceSHA256 = in.SourceSHA256
		o.State.Visibility = in.Config.Repo.Visibility
		configState := &ConfigState{Source: in.ConfigSource, Persisted: in.ConfigPersisted, SHA256: in.ConfigSHA256}
		if in.ConfigPersisted {
			configState.Path = in.ConfigPath
		}
		o.State.Config = configState
		o.State.enter("PLAN")
	}
	return in, nil
}

// explainUnresolvedSource decides what an unselectable source means.
//
// Ambiguity is always a safety decision: interactive Simple Mode resolves it
// before planning, while machine and non-interactive callers get a hard
// SOURCE_AMBIGUOUS result rather than a guessed source. Only a human at a
// terminal is shown guidance instead of an error.
func explainUnresolvedSource(o Options, source sourceSelection, err error) error {
	var ambiguous *sourceAmbiguityError
	if errors.As(err, &ambiguous) || (source.Discovery != nil && source.Discovery.NeedsInput) {
		return err
	}
	if o.ReadOnly || o.JSON {
		return err
	}
	fmt.Printf("GitMake %s\n\n", Version)
	fmt.Println("No project source could be selected in this folder.")
	fmt.Println("\nRun GitMake inside a project folder, or provide a source directly:")
	fmt.Println("  gitmake .")
	fmt.Println("  gitmake path\\to\\Project.zip")
	return errSourceGuidanceShown
}
