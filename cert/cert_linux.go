package cert

import "fmt"

const supportDir = "~/.zap/ssl"

// InstallCert installs a CA certificate root in the system cacerts on linux
func InstallCert(cert string) error {
	fmt.Printf("! Add %s to your browser to trust CA\n", cert)
	return nil

	// determine if we're running Ubuntu (and what version) with
	// lsb_release -a
	// cat /etc/*release
}

// https://thomas-leister.de/en/how-to-import-ca-root-certificate/

// #!/bin/bash

// ### Script installs root.cert.pem to certificate trust store of applications using NSS
// ### (e.g. Firefox, Thunderbird, Chromium)
// ### Mozilla uses cert8, Chromium and Chrome use cert9

// ###
// ### Requirement: apt install libnss3-tools
// ###

// ###
// ### CA file to install (CUSTOMIZE!)
// ###

// certfile="cert.pem"
// certname="ZAP CA"

// ###
// ### For cert8 (legacy - DBM)
// ###

// for certDB in $(find ~/ -name "cert8.db")
// do
//     certdir=$(dirname ${certDB});
//     certutil -A -n "${certname}" -t "TCu,Cu,Tu" -i ${certfile} -d dbm:${certdir}
// done

// ###
// ### For cert9 (SQL)
// ###

// for certDB in $(find ~/ -name "cert9.db")
// do
//     certdir=$(dirname ${certDB});
//     certutil -A -n "${certname}" -t "TCu,Cu,Tu" -i ${certfile} -d sql:${certdir}
// done
