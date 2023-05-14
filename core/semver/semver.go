package semver

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/catalystsquad/app-utils-go/logging"
)

type CommitType int

const (
	NotConventional CommitType = iota
	Patch
	Minor
	Major
)

type Semver struct {
	Major      uint32
	Minor      uint32
	Patch      uint32
	PreRelease string
	Build      string
}

func NewSemver(major, minor, patch uint32) *Semver {
	return &Semver{major, minor, patch, "", ""}
}

func (v *Semver) Clone() *Semver {
	retVal := NewSemver(v.Major, v.Minor, v.Patch)
	retVal.PreRelease = v.PreRelease
	retVal.Build = v.Build
	return retVal
}

// The only "logic" function, you have to pass in everything that could matter and it will bump appropriately
func (v *Semver) BumpVersion(commitType CommitType, preRelease string, build string) {
	logging.Log.Info(fmt.Sprintf("Bumping: %s\nBased on (type,pre-release,build): %d, %s, %s\n", v.FormattedString(), commitType, preRelease, build))
	cleanPreRelease := strings.Trim(preRelease, " \n\r\t")
	currentPreRelease := strings.Split(v.PreRelease, ".")[0]

	logging.Log.Info(fmt.Sprintf("Clean PreRelease: %s\n", cleanPreRelease))
	logging.Log.Info(fmt.Sprintf("Current PreRelease: %s\n", currentPreRelease))
	if cleanPreRelease == currentPreRelease {
		v.IncrementPreRelease()
		return
	}
	// The current prerelease is not the same, so just set this one instead of incrementing Major/Minor/Patch
	if cleanPreRelease != "" {
		v.PreRelease = preRelease + ".1"
		logging.Log.Info(fmt.Sprintf("Setting PreRelease: %s\n", v.PreRelease))
		if build != "" {
			v.Build = build
		}
		return
	}

	switch commitType {
	case Patch:
		v.BumpPatch()
	case Minor:
		v.BumpMinor()
	case Major:
		v.BumpMajor()
	case NotConventional:
		return
	default:
		return
	}
	if build != "" {
		v.Build = build
	}
}

func (v *Semver) BumpMajor() {
	v.Major += 1
	v.Minor = 0
	v.Patch = 0
	v.PreRelease = ""
	v.Build = ""
}

func (v *Semver) BumpMinor() {
	v.Minor += 1
	v.Patch = 0
	v.PreRelease = ""
	v.Build = ""
}

func (v *Semver) BumpPatch() {
	v.Patch += 1
	v.PreRelease = ""
	v.Build = ""
}

func (v *Semver) IncrementPreRelease() {
	parts := strings.Split(v.PreRelease, ".")
	if len(parts) < 2 {
		v.PreRelease = parts[0] + ".2"
		return
	}

	numberPart := parts[1]
	number, err := strconv.Atoi(numberPart)
	// If there's an error, assume it's a string and not an int, and make it 1
	if err != nil {
		v.PreRelease = parts[0] + ".1"
		return
	}

	v.PreRelease = parts[0] + "." + fmt.Sprintf("%d", number+1)
}

func (v *Semver) FormattedString() string {
	retVal := "v"

	retVal += fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)

	if v.PreRelease != "" {
		retVal += fmt.Sprintf("-%s", v.PreRelease)
	}
	if v.Build != "" {
		retVal += fmt.Sprintf("+%s", v.Build)
	}

	return retVal
}
