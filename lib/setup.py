from setuptools import setup


with open("README.md", "r") as readme_file:
    readme_content = readme_file.read()

with open("VERSION", "r") as version_file:
    version = version_file.read().split('=')[1].strip()

setup(
    name='minimega',
    version=version,
    author="minimega dev team",
    author_email="minimega-dev@sandia.gov",
    description="Python API for minimega",
    long_description=readme_content,
    long_description_content_type="text/markdown",
    license="GPLv3",
    url="https://www.sandia.gov/minimega/",
    project_urls={
        "homepage": "https://www.sandia.gov/minimega/",
        "repository": "https://github.com/sandia-minimega/minimega",
        "changelog": "https://github.com/sandia-minimega/minimega/releases/",
        "documentation": "https://www.sandia.gov/minimega/using-minimega/",
        "issues": "https://github.com/sandia-minimega/minimega/issues",
    },
    py_modules=["minimega"],
    python_requires=">=3.6",
    classifiers=[
        "Development Status :: 5 - Production/Stable",
        "Environment :: Console",
        "Intended Audience :: Developers",
        "Intended Audience :: Information Technology",
        "Intended Audience :: Telecommunications Industry",
        "Intended Audience :: Science/Research",
        "License :: OSI Approved :: GNU General Public License v3 (GPLv3)",
        "Natural Language :: English",
        "Operating System :: OS Independent",
        "Programming Language :: Python",
        "Programming Language :: Python :: 3",
        "Programming Language :: Python :: 3.6",
        "Programming Language :: Python :: 3.7",
        "Programming Language :: Python :: 3.8",
        "Programming Language :: Python :: 3.9",
        "Programming Language :: Python :: 3.10",
        "Programming Language :: Python :: Implementation :: CPython",
        "Topic :: Internet",
        "Topic :: Scientific/Engineering",
        "Topic :: System :: Clustering",
        "Topic :: System :: Distributed Computing",
        "Topic :: System :: Emulators",
        "Topic :: System :: Networking",
        "Topic :: System :: Operating System",
        "Topic :: Utilities",
    ],
)
