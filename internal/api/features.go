package api

// builtFeatures names the parts of controller moving that this build holds in
// full. The list is read by GET /api/version so a script driving a controller
// can ask what it can rely on, and an older controller, which has no list,
// answers with no "features" field at all.
//
// A name is added in the same change that finishes the last piece of its part,
// and never before: "fence", "copy-origin", "restore-public-url",
// "copy-password-request", "fence-address" (the address can be changed while
// fenced) and "fence-machines" (the fence reports which machines have
// followed) are the names the contract reserves.
var builtFeatures = []string{"copy-origin", "fence", "restore-public-url", "copy-password-request", "fence-address", "fence-machines"}

// featureList is what the version document reports: the built parts, or nil
// when there are none so the field is left out rather than sent empty.
func featureList() []string {
	if len(builtFeatures) == 0 {
		return nil
	}
	return append([]string(nil), builtFeatures...)
}
