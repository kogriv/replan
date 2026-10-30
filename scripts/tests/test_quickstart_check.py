import unittest

import quickstart_check


class BlocksTest(unittest.TestCase):
    def test_text_block_after_sh_block_is_its_expected_output(self):
        page = (
            "Intro\n\n```sh\nreplan init x\n```\n\n```text\nline one\n\nline two\n```\n\n"
            "```bash\nnot run\n```\n\n```sh\nreplan validate .\n```\n\n```text\nlate\n```\n"
        )
        self.assertEqual(
            quickstart_check.blocks(page),
            [("replan init x\n", ["line one", "line two"]), ("replan validate .\n", ["late"])],
        )

    def test_text_block_without_a_command_is_ignored(self):
        self.assertEqual(quickstart_check.blocks("```text\nalone\n```\n"), [])

    def test_spaces_compare_loosely(self):
        self.assertEqual(quickstart_check.squeeze("  a   b  c "), "a b c")


if __name__ == "__main__":
    unittest.main()
