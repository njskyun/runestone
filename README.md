# 起源
这是一位大牛写的大饼链上打符文的脚本，发现在分型上有些问题，把整个fork下来，进行了改动，已支持分型网络，且专门针对已经拆分了utxo的地址更加友好


## 前提

已经安装fractalbitcoin全节点

## 安装
 
```go
wget https://golang.org/dl/go1.23.1.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.23.1.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
go version

git clone https://github.com/njskyun/runestone.git
cd runestone/
go mod tidy
```
### 使用
1. cd cmd/runestonecli
2. 修改config.yaml配置文件
3. main.go 文件中有一些基本逻辑，可以自行更改
4. 运行：go run .

  

### Reference:

* https://docs.ordinals.com/runes/specification.html
* https://github.com/ordinals/ord/
* https://github.com/bxelab/runestone
