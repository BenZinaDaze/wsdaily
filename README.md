## Go 版本武神日常

### 配置

    ./conf.yaml
    cron 定时  Cron语法
    logins 账号组
        -login 账号1
         password 密码1
         server 区 1 2 3 4 99 100
        -login 账号2
         password 密码2
         server 区 1 2 3 4 99 100

### 编译

    make build os=windows arch=amd64
    参数:
        os：操作系统 默认windows
            可选参数：darwin,freebsd,linux,windows
        arch：架构 默认amd64
            可选参数：386,amd64,arm
    例如：
        编译MAC可执行程序
        make build os=darwin
        编译linux arm可执行程序
        make build os=linux arch=arm
