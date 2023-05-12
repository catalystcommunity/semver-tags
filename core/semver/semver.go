package semver

import (
	"fmt"
	"strconv"
	"strings"
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

func (v *Semver) IncrementPreRelease() int {
	parts := strings.Split(v.PreRelease, ".")
	if len(parts) < 2 {
		return 1
	}

	numberPart := parts[1]
	number, err := strconv.Atoi(numberPart)
	if err != nil {
		return 1
	}

	return number + 1
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
