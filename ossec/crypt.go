/*
see: https://documentation.wazuh.com/4.0/development/message-format.html

*/
package ossec

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"fmt"
	"io"
	"math/rand"
	"strings"

	"github.com/4kills/go-zlib"
	// "compress/zlib"

	"golang.org/x/crypto/blowfish"
)

func blowfishEncrypt(ppt, key []byte) []byte {
	// create the cipher
	bfCipher, err := blowfish.NewCipher(key)
	if err != nil {
		// fix this. its okay for this tester program, but ....
		panic(err)
	}

	// make ciphertext big enough to store len(ppt)
	ciphertext := make([]byte, len(ppt))

	// create the encrypter
	ecbc := cipher.NewCBCEncrypter(bfCipher, []byte{0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10})
	// encrypt the blocks, because block cipher
	ecbc.CryptBlocks(ciphertext, ppt)
	// return ciphertext to calling function
	return ciphertext
}

func blowfishDecrypt(ppt, key []byte) []byte {
	// create the cipher
	bfCipher, err := blowfish.NewCipher(key)
	if err != nil {
		// fix this. its okay for this tester program, but ....
		panic(err)
	}
	// pad
	pad := bfCipher.BlockSize() - (len(ppt) % bfCipher.BlockSize())
	for pad > 0 && pad < bfCipher.BlockSize() {
		ppt = append(ppt, 0)
		pad--
	}

	// make ciphertext big enough to store len(ppt)
	ciphertext := make([]byte, len(ppt))

	// create the encrypter
	ecbc := cipher.NewCBCDecrypter(bfCipher, []byte{0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10})

	ecbc.CryptBlocks(ciphertext, ppt)
	return ciphertext
}

// Use PKCS7 to fill, IOS is also 7
func PKCS7Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padtext...)
}

func PKCS7UnPadding(origData []byte) []byte {
	length := len(origData)
	unpadding := int(origData[length-1])
	return origData[:(length - unpadding)]
}

func aesEncrypt(ppt, key []byte) []byte {
	// ppt = []byte("TestString123456TestString123456")
	// create the cipher
	aesCipher, err := aes.NewCipher(key[:32])
	if err != nil {
		// fix this. its okay for this tester program, but ....
		panic(err)
	}

	// create the encrypter
	// fmt.Println(aesCipher.BlockSize())
	ivBytes := []byte("FEDCBA0987654321")
	ecbc := cipher.NewCBCEncrypter(aesCipher, ivBytes)

	ppt = PKCS7Padding(ppt, ecbc.BlockSize())

	// make ciphertext big enough to store len(ppt)
	// ciphertext := make([]byte, len(ppt)+ecbc.BlockSize())
	ciphertext := make([]byte, len(ppt))

	// encrypt the blocks, because block cipher
	ecbc.CryptBlocks(ciphertext, ppt)
	// ecbc.CryptBlocks(ciphertext[len(ppt):], ivBytes)

	// return ciphertext to calling function
	return ciphertext
}

func aesDecrypt(ppt, key []byte) []byte {

	// create the cipher
	aesCipher, err := aes.NewCipher(key[:32])
	if err != nil {
		// fix this. its okay for this tester program, but ....
		panic(err)
	}
	// pad
	pad := aesCipher.BlockSize() - (len(ppt) % aesCipher.BlockSize())
	for pad > 0 && pad < aesCipher.BlockSize() {
		ppt = append(ppt, 0)
		pad--
	}

	// make ciphertext big enough to store len(ppt)
	ciphertext := make([]byte, len(ppt))

	// create the encrypter
	ecbc := cipher.NewCBCDecrypter(aesCipher, []byte("FEDCBA0987654321"))

	ecbc.CryptBlocks(ciphertext, ppt)
	return ciphertext

}

func (a *Client) decryptMessage(encMsg []byte, msgSize uint32) (string, error) {
	var compressed []byte

	if encMsg[0] == '!' {
		endAgentID := strings.Index(string(encMsg[1:]), "!")
		if endAgentID == -1 {
			return "", NewCorruptMessage("missing exclamation mark")
		}
		agentID := string(encMsg[1 : endAgentID+1])
		if agentID != a.AgentID {
			return "", NewCorruptMessage("AgentID not matching")
		}
		msgSize = msgSize - (uint32(endAgentID) + 2)
		encMsg = encMsg[endAgentID+2:]
	}

	method := EncryptionMethodBlowFish
	if string(encMsg[:4]) == "#AES" {
		method = EncryptionMethodAES
		encMsg = encMsg[4:]
		msgSize = msgSize - 4
	}
	if encMsg[0] != ':' {
		return "", NewCorruptMessage("missing colon")
	}
	encMsg = encMsg[1:]
	msgSize--

	if int(msgSize) > len(encMsg) {
		return "", NewCorruptMessage("invalid decrypted length")
	}

	if method == EncryptionMethodBlowFish {
		compressed = blowfishDecrypt([]byte(encMsg[0:msgSize]), []byte(a.AgentHashedKey))
	} else {
		compressed = aesDecrypt([]byte(encMsg[0:msgSize]), []byte(a.AgentHashedKey))
	}
	for compressed[0] == '!' {
		compressed = compressed[1:]
		msgSize--
	}
	// fmt.Printf("%0x %s\n", compressed, string(compressed))
	b := bytes.NewReader(compressed[:msgSize])

	r, err := zlib.NewReader(b)
	if err != nil {
		return "", err
	}
	defer r.Close()

	buf := make([]byte, 1024)

	// _, err = io.Copy(&w, r)
	read, err := r.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}
	msg := buf[:read]

	return string(msg), nil
}

func (a *Client) cryptMsg(msg string) ([]byte, uint32) {
	msgSize := uint(len(msg))
	/* Random number, take only 5 chars ~= 2^16=65536*/
	rand1 := rand.Intn(65536)

	/* Increase local and global counters */
	if a.localCount >= 9997 {
		a.localCount = 0
		a.globalCount++
	}
	a.localCount++

	tmpMsg := fmt.Sprintf("%05d%010d:%04d:%s",
		rand1, a.globalCount, a.localCount,
		msg)

	/* Generate MD5 of the unencrypted string */

	md5Sum := fmt.Sprintf("%x", md5.Sum([]byte(tmpMsg)))
	// fmt.Printf("_tmpmsg md5: %s\n", md5Sum)
	finMsg := fmt.Sprintf("%s%s", md5Sum, tmpMsg)
	// fmt.Printf("_finMsg: '%s'\n", finMsg)
	/* Compress the message
	* We assign the first 8 bytes for padding
	 */
	var b bytes.Buffer
	w, err := zlib.NewWriterLevel(&b, 9)
	if err != nil {
		return nil, 0
	}
	// , _ :=
	w.Write([]byte(finMsg))

	w.Close()
	compressedMsg := b.Bytes()
	cmpSize := uint(len(compressedMsg))
	// fmt.Printf("_tmpMsg: %d:%d:%d ->  '%s'\n", len([]byte(finMsg)), written, cmpSize, tmpMsg)

	/* Pad the message (needs to be div by 8) */
	bfSize := 8 - (cmpSize % 8)
	if bfSize == 8 {
		bfSize = 0
	}

	// fmt.Printf("compressed: <%s>: %s (%d -> %d)\n", fmt.Sprintf("%00x", md5.Sum([]byte(compressedMsg))), compressedMsg, written, cmpSize)
	// fmt.Printf("%00x", []byte(compressedMsg))
	// fmt.Printf("\n")

	tmpMsg = fmt.Sprintf("%s%s", "!!!!!!!!"[:bfSize], string(compressedMsg))
	cmpSize += bfSize
	// fmt.Printf("tmpMsg:  '%s' (%d)\n", tmpMsg, len(tmpMsg))

	/* Get average sizes */
	a.cOrigSize += msgSize
	a.cCompSize += cmpSize

	var cryptoToken string
	var encrypted []byte
	if a.EncryptionMethod == EncryptionMethodAES {
		cryptoToken = "#AES:"
		encrypted = aesEncrypt([]byte(tmpMsg), []byte(a.AgentHashedKey))
	} else {
		cryptoToken = ":"
		encrypted = blowfishEncrypt([]byte(tmpMsg), []byte(a.AgentHashedKey))
	}
	var msgEncrypted string
	if a.AgentAllowedIPs == "any" {
		msgEncrypted = fmt.Sprintf("!%s!%s%s", a.AgentID, cryptoToken, encrypted)
	} else {
		msgEncrypted = fmt.Sprintf("%s%s", cryptoToken, encrypted)
	}

	if cmpSize < uint(len(msgEncrypted)) {
		cmpSize = uint(len(msgEncrypted))
	}

	// fmt.Printf("encMsg:  '%s' (%d)\n", msgEncrypted, cmpSize)
	// decrypted, err := a.decryptMessage([]byte(msgEncrypted), (uint32)(cmpSize))

	// fmt.Printf("decMsg:  '%s' (%v)\n", decrypted, err)
	return []byte(msgEncrypted), (uint32)(cmpSize)
}

type CorruptMessage struct {
	typ string
}

func NewCorruptMessage(typ string) CorruptMessage {
	return CorruptMessage{
		typ: typ,
	}
}
func (cme CorruptMessage) Error() string {
	return fmt.Sprintf("corrupt message (%s)", cme.typ)
}
