package mutagen

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"go.jetpack.io/devbox/internal/debug"
)

func Create(spec *SessionSpec) error {
	err := spec.Validate()
	if err != nil {
		return err
	}

	alpha := spec.AlphaPath
	if spec.AlphaAddress != "" {
		alpha = fmt.Sprintf("%s:%s", spec.AlphaAddress, spec.AlphaPath)
	}

	beta := spec.BetaPath
	if spec.BetaAddress != "" {
		beta = fmt.Sprintf("%s:%s", spec.BetaAddress, spec.BetaPath)
	}

	args := []string{"sync", "create", alpha, beta}
	if spec.Name != "" {
		args = append(args, "--name", spec.Name)
	}
	if spec.Paused {
		args = append(args, "--paused")
	}

	for k, v := range spec.Labels {
		args = append(args, "--label", fmt.Sprintf("%s=%s", k, v))
	}

	if spec.SyncMode == "" {
		args = append(args, "--sync-mode", "two-way-resolved")
	} else {
		args = append(args, "--sync-mode", spec.SyncMode)
	}

	if spec.IgnoreVCS {
		args = append(args, "--ignore-vcs")
	}

	return execMutagen(args, spec.EnvVars)
}

func List(envVars map[string]string, names ...string) ([]Session, error) {
	binPath := ensureMutagen()
	cmd := exec.Command(binPath, "sync", "list", "--template", "{{json .}}")
	cmd.Args = append(cmd.Args, names...)
	cmd.Env = envAsKeyValueStrings(envVars)

	debugPrintExecCmd(cmd)
	out, err := cmd.CombinedOutput()

	if err != nil {
		debug.Log("List error: %s, and out: %s", err, string(out))
		if e := (&exec.ExitError{}); errors.As(err, &e) {
			errMsg := strings.TrimSpace(string(out))
			// Special handle the case where no sessions are found:
			if strings.Contains(errMsg, "unable to locate requested sessions") {
				return []Session{}, nil
			}
			return nil, errors.New(errMsg)
		}
		return nil, err
	}

	sessions := []Session{}
	err = json.Unmarshal(out, &sessions)
	if err != nil {
		return nil, err
	}

	return sessions, nil
}

func Pause(names ...string) error {
	args := []string{"sync", "pause"}
	args = append(args, names...)
	return execMutagen(args, nil /*envVars*/)
}

func Resume(envVars map[string]string, names ...string) error {
	args := []string{"sync", "resume"}
	args = append(args, names...)
	return execMutagen(args, envVars)
}

func Flush(names ...string) error {
	args := []string{"sync", "flush"}
	args = append(args, names...)
	return execMutagen(args, nil /*envVars*/)
}

func Reset(envVars map[string]string, names ...string) error {
	args := []string{"sync", "reset"}
	args = append(args, names...)
	return execMutagen(args, envVars)
}

func Terminate(env map[string]string, labels map[string]string, names ...string) error {
	args := []string{"sync", "terminate"}

	for k, v := range labels {
		args = append(args, "--label-selector", fmt.Sprintf("%s=%s", k, v))
	}

	args = append(args, names...)
	return execMutagen(args, env)
}

func execMutagen(args []string, envVars map[string]string) error {
	binPath := ensureMutagen()
	cmd := exec.Command(binPath, args...)
	cmd.Env = envAsKeyValueStrings(envVars)

	debugPrintExecCmd(cmd)
	out, err := cmd.CombinedOutput()

	if err != nil {
		debug.Log("execMutagen error: %s, out: %s", err, string(out))
		if e := (&exec.ExitError{}); errors.As(err, &e) {
			return errors.New(strings.TrimSpace(string(out)))
		}
		return err
	}

	debug.Log("execMutagen worked for cmd: %s", cmd)
	return nil
}

// debugPrintExecCmd prints the command to be run, along with MUTAGEN env-vars
func debugPrintExecCmd(cmd *exec.Cmd) {
	envPrint := ""
	for _, cmdEnv := range cmd.Env {
		if strings.HasPrefix(cmdEnv, "MUTAGEN") {
			envPrint = fmt.Sprintf("%s, %s", envPrint, cmdEnv)
		}
	}
	debug.Log("running mutagen cmd %s with MUTAGEN env: %s", cmd.String(), envPrint)
}

// envAsKeyValueStrings prepares the env-vars in key=value format to add to the command to be run
//
// panics if os.Environ() returns an array with any element not in key=value format
func envAsKeyValueStrings(userEnv map[string]string) []string {
	if userEnv == nil {
		userEnv = map[string]string{}
	}

	// Convert env to map, and strip out MUTAGEN_PROMPTER env-var
	envMap := map[string]string{}
	for _, envVar := range os.Environ() {
		k, v, found := strings.Cut(envVar, "=")
		if !found {
			panic(fmt.Sprintf("did not find an = in env-var: %s", envVar))
		}
		// Mutagen sets this variable for ssh/scp scenarios, which then expect interactivity?
		// https://github.com/mutagen-io/mutagen/blob/b97ff3764a6a6cb91b48ad27def078f6d6a76e24/cmd/mutagen/main.go#L89-L94
		//
		// We do not include MUTAGEN_PROMPTER, otherwise mutagen-CLI rejects the command we are about to invoke,
		// by treating it instead as a prompter-command.
		if k != "MUTAGEN_PROMPTER" {
			envMap[k] = v
		}
	}

	// userEnv overrides the default env
	for k, v := range userEnv {
		envMap[k] = v
	}

	// Convert the envMap to an envList
	envList := make([]string, 0, len(envMap))
	for k, v := range envMap {
		envList = append(envList, fmt.Sprintf("%s=%s", k, v))
	}
	return envList
}

func ensureMutagen() string {
	installPath := CacheSubpath("mutagen/bin/mutagen")
	err := InstallMutagenOnce(installPath)
	if err != nil {
		panic(err)
	}
	return installPath
}