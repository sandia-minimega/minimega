from setuptools import setup


setup(
    name="minimega-pygments",
    version="0.1.0",
    py_modules=["minimega_lexer"],
    install_requires=["Pygments>=2.16"],
    entry_points={
        "pygments.lexers": [
            "minimega = minimega_lexer:MinimegaLexer",
        ],
    },
)
