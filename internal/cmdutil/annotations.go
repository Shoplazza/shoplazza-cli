package cmdutil

// AnnotationNotScannable marks a cobra command that blind CLI scans (the
// contract smoke suite) must skip: interactive, long-running, or writes the
// local filesystem. Stamp it at the definition site; it covers the subtree.
const AnnotationNotScannable = "shoplazza.notscannable"
