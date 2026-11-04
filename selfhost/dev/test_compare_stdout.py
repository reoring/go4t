import unittest

from compare_stdout import compare


class CompareStdoutTests(unittest.TestCase):
    def test_deterministic_output_is_exact(self):
        self.assertIsNone(compare(b'a\nb\n', b'a\nb\n'))
        self.assertIn('line 2', compare(b'a\nb\n', b'a\nc\n'))

    def test_explicit_nondeterminism_compares_only_line_counts(self):
        self.assertIsNone(compare(b'1\nstable\n', b'3\nstable\n', nondeterministic=True))
        self.assertIsNone(compare(b'1\nstable\n', b'3\nchanged\n', nondeterministic=True))

    def test_all_line_counts_must_match(self):
        self.assertIn('line counts', compare(b'a\nb\n', b'a\n', nondeterministic=True))
        self.assertIn('line counts', compare(b'a\n', b'a\nb\n'))

    def test_line_endings_remain_observable(self):
        self.assertIsNotNone(compare(b'a\n', b'a'))
        self.assertIsNotNone(compare(b'a\r\n', b'a\n'))

    def test_empty_output(self):
        self.assertIsNone(compare(b'', b''))


if __name__ == '__main__':
    unittest.main()
