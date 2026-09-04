import unittest

from pygments.token import Comment, Keyword, Name

from minimega_lexer import MinimegaLexer


class MinimegaLexerTest(unittest.TestCase):
    def test_command_tokens(self):
        tokens = list(
            MinimegaLexer().get_tokens(
                ".columns name,state vm info\n"
                "namespace example\n"
                "vm launch kvm node1 # create VM\n"
            )
        )

        self.assertIn((Name.Builtin, ".columns"), tokens)
        self.assertIn((Keyword, "vm"), tokens)
        self.assertIn((Name.Function, "launch"), tokens)
        self.assertIn((Comment.Single, "# create VM"), tokens)


if __name__ == "__main__":
    unittest.main()
