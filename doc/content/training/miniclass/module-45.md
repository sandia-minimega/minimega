# Module 45 - Trusted Platform Module (TPM)

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-45-trusted-platform-module-tpm/).

## Introduction

VMs can be configured to use a virtual [Trust Platform Module (TPM)](https://en.wikipedia.org/wiki/Trusted_Platform_Module#:~:text=Trusted%20Platform%20Module%20(TPM%2C%20also,chip%20conforming%20to%20the%20standard.) during configuration. A TPM provides a variety of security features such as the generation and storage of cryptographic keys. Note that a virtual TPM does not have the same security guarantees as a true hardware module (see [here](https://github.com/stefanberger/swtpm/wiki#securitytrust-model-of-the-software-tpm)).

Additional dependencies are required to create the virtual TPM socket the VM connects to. The following sections describe the use of one option, swtpm.

### Using swtpm

[sw`tpm`](https://github.com/stefanberger/swtpm) is a software TPM emulator that has been tested with minimega. The general process for using swtpm is as follows:

1. Install swtpm on the host machine following the [instructions](https://github.com/stefanberger/swtpm/wiki#compile-and-install-on-linux)
2. Start a swtpm socket: `swtpm socket --tpmstate dir=/mydir --ctrl type=unixio,path=/mydir/swtpm-socket`
3. Configure your VM to point to the socket `vm config tpm-socket /mydir/swtpm-socket`
4. After booting, configure your VM as needed to utilize TPM
   1. For example, a Windows domain can be configured to use [virtual smartcards](https://learn.microsoft.com/en-us/windows/security/identity-protection/virtual-smart-cards/virtual-smart-card-get-started)

Repeat steps 2-4 for each individual VM that needs a TPM. A different socket should be used per VM.
