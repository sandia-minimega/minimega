# Module 4 - Installing Minimega

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-4-installing-minimega/).

## Introduction

Minimega can be built from latest source or deployed from a release package.

## Installing latest Minimega

Note: These will have bleeding edge updates and fixes

### Building Minimega with git

```
cd /home/ubuntu/
git clone https://github.com/sandia-minimega/minimega.git
cd minimega
./all.bash
```

### Building Minimega with wget

```
cd /home/ubuntu/
wget https://github.com/sandia-minimega/minimega/archive/master.zip
unzip master.zip
mv minimega-master minimega
cd minimega
./all.bash
```

## Installing Minimega from Releases

Note: These will not have the latest updates and fixes

### Installing Minimega with dpkg

Note: This will install minimega to /opt/minimega/

```
wget https://github.com/sandia-minimega/minimega/releases/download/2.7/minimega-2.7.deb
dpkg -i minimega-2.7.deb
```

### Installing Minimega with binary tarball

```
cd /home/ubuntu/
wget https://github.com/sandia-minimega/minimega/releases/download/2.7/minimega-2.7-binaries.tar.bz2
tar xjf minimega-2.7-binaries.tar.bz2
cd minimega
```

### Adding minimega to the PATH Variable

```
echo "export PATH=$PATH:/home/ubuntu/minimega/bin">>~/.bashrc
source ~/.bashrc
```
