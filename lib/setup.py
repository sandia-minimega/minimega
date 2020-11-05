import os
import shutil
from distutils.core import setup


with open("README", "r") as readme_file:
    readme_content = readme_file.read()

setup(
    name='minimega',
    description="Python API for minimega",
    author="minimega dev team",
    author_email="minimega-dev@sandia.gov",
    long_description=readme_content,
    license="GPLv3",
    url="https://minimega.org",
    version="2.7",
    py_modules=["minimega"],
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
