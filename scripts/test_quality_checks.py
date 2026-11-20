import unittest

from quality_diff import added, coverage, invalid_directive
from tmux_matrix_check import compare, expected


class MatrixTest(unittest.TestCase):
    base = '  tmux 3.5 ✓\n  tmux 3.6a ✓'
    versions = expected([{'tag_name': tag} for tag in ['2.8', '3.5', '3.6a', '3.7-rc']])

    def test_complete(self):
        self.assertEqual(compare(self.base, self.base, self.versions), [])
        self.assertEqual(compare(self.base, self.base.replace('3.5 ✓', '3.5 ✗'), self.versions), ['3.5'])

    def test_invalid(self):
        for base, head in [(self.base, '  tmux 3.5 ✓'), (self.base.replace('✓', '✗'), self.base),
                           (self.base, self.base + '\n  tmux 3.5 ✓'), ('', '')]:
            with self.subTest(base=base, head=head), self.assertRaises(ValueError):
                compare(base, head, self.versions)


class DiffTest(unittest.TestCase):
    def test_blocks(self):
        changes = list(added('diff --git a/a.go b/a.go\n+++ b/a.go\n@@ -2,0 +3,3 @@\n+x\n+y\n+z'))
        profile = 'mode: atomic\nexample/a.go:2.1,3.2 2 1\nexample/a.go:4.1,4.9 1 0\n'
        self.assertEqual(coverage(changes, profile, 'example'),
                         'Changed Go lines intersecting coverage blocks: 1/2 covered.\na.go:4')

    def test_nolint(self):
        for invalid in ['//nolint', '//nolint:gosec', '//nolint:gosec,all // why']:
            self.assertTrue(invalid_directive(invalid))
        self.assertFalse(invalid_directive('call() //nolint:gosec // test fixture'))

    def test_comment_tokens(self):
        from quality_diff import comments
        code = 'var a = `line\n//nolint`\nf("//nolint") //nolint:demo // justified\n/* //nolint */'
        self.assertEqual(list(comments(code)), [(3, '//nolint:demo // justified')])

    def test_quoted_path(self):
        diff = 'diff --git a/file b/file\n+++ "b/space\\tfile.go"\n@@ -0,0 +1 @@\n+package example'
        self.assertEqual(list(added(diff)), [('space\tfile.go', 1, 'package example')])
