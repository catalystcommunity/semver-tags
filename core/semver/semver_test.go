package semver

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBumpMajor(t *testing.T) {
	version := NewSemver(0, 1, 0)

	version.BumpMajor()
	assert.Equal(t, NewSemver(1, 0, 0), version, "bumpMajor did not give correct 1st bump result")

	version.BumpMajor()
	assert.Equal(t, NewSemver(2, 0, 0), version, "bumpMajor did not give correct 2nd bump result")
}

func TestBumpMinor(t *testing.T) {
	version := NewSemver(0, 1, 0)

	version.BumpMinor()
	assert.Equal(t, NewSemver(0, 2, 0), version, "bumpMinor did not give correct 1st bump result")

	version.BumpMinor()
	assert.Equal(t, NewSemver(0, 3, 0), version, "bumpMinor did not give correct 2nd bump result")
}

func TestBumpPatch(t *testing.T) {
	version := NewSemver(0, 1, 0)

	version.BumpPatch()
	assert.Equal(t, NewSemver(0, 1, 1), version, "bumpPatch did not give correct 1st bump result")

	version.BumpPatch()
	assert.Equal(t, NewSemver(0, 1, 2), version, "bumpPatch did not give correct 2nd bump result")
}

func TestBumpVersionKeepsVersionForNotConventional(t *testing.T) {
	version := NewSemver(1, 2, 3)

	version.BumpVersion(NotConventional, "", "")

	assert.Equal(t, "v1.2.3", version.FormattedString())
}

func TestBumpVersionSetsBuildString(t *testing.T) {
	version := NewSemver(1, 2, 3)

	version.BumpVersion(Patch, "", "abc123")

	assert.Equal(t, "v1.2.4+abc123", version.FormattedString())
}

func TestBumpVersionStartsAndIncrementsPreRelease(t *testing.T) {
	version := NewSemver(1, 2, 3)

	version.BumpVersion(Patch, "rc", "")
	assert.Equal(t, "v1.2.3-rc.1", version.FormattedString())

	version.BumpVersion(Patch, "rc", "")
	assert.Equal(t, "v1.2.3-rc.2", version.FormattedString())
}

func TestFormattedString(t *testing.T) {
	version := &Semver{Major: 1, Minor: 2, Patch: 3, PreRelease: "rc.1", Build: "abc"}

	assert.Equal(t, "v1.2.3-rc.1+abc", version.FormattedString())
}

func parse(t *testing.T, major, minor, patch uint32, preRelease string) *Semver {
	t.Helper()
	version := NewSemver(major, minor, patch)
	version.PreRelease = preRelease
	return version
}

func TestCompareUsesVersionNumbers(t *testing.T) {
	assert.Equal(t, -1, NewSemver(1, 0, 0).Compare(NewSemver(2, 0, 0)))
	assert.Equal(t, 1, NewSemver(2, 0, 0).Compare(NewSemver(1, 9, 9)))
	assert.Equal(t, -1, NewSemver(1, 1, 0).Compare(NewSemver(1, 2, 0)))
	assert.Equal(t, -1, NewSemver(1, 1, 1).Compare(NewSemver(1, 1, 2)))
	assert.Equal(t, 0, NewSemver(1, 2, 3).Compare(NewSemver(1, 2, 3)))
}

// The build part does not change precedence, so two versions that differ only
// in the build part are the same.
func TestCompareIgnoresBuild(t *testing.T) {
	left := NewSemver(1, 2, 3)
	left.Build = "one"
	right := NewSemver(1, 2, 3)
	right.Build = "two"

	assert.Equal(t, 0, left.Compare(right))
}

func TestComparePutsPreReleaseBelowItsRelease(t *testing.T) {
	assert.Equal(t, -1, parse(t, 1, 0, 0, "rc.1").Compare(NewSemver(1, 0, 0)))
	assert.Equal(t, 1, NewSemver(1, 0, 0).Compare(parse(t, 1, 0, 0, "rc.1")))
}

func TestComparePreReleaseIdentifiers(t *testing.T) {
	// Numbers compare as numbers, not as text
	assert.Equal(t, -1, parse(t, 1, 0, 0, "rc.2").Compare(parse(t, 1, 0, 0, "rc.10")))
	// A number is lower than text
	assert.Equal(t, -1, parse(t, 1, 0, 0, "1").Compare(parse(t, 1, 0, 0, "alpha")))
	// Text compares as text
	assert.Equal(t, -1, parse(t, 1, 0, 0, "alpha").Compare(parse(t, 1, 0, 0, "beta")))
	// More parts is higher when the common parts are the same
	assert.Equal(t, -1, parse(t, 1, 0, 0, "rc").Compare(parse(t, 1, 0, 0, "rc.1")))
	assert.Equal(t, 0, parse(t, 1, 0, 0, "rc.1").Compare(parse(t, 1, 0, 0, "rc.1")))
}

// A higher version on an older commit must still win, which is what the tag
// search needs.
func TestCompareOrdersAReleaseSeries(t *testing.T) {
	assert.Equal(t, 1, NewSemver(2, 0, 0).Compare(NewSemver(1, 1, 0)))
	assert.Equal(t, 1, parse(t, 2, 0, 0, "rc.1").Compare(NewSemver(1, 9, 9)))
}
