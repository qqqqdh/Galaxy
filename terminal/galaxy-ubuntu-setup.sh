#!/bin/bash
set -eu
export DEBIAN_FRONTEND=noninteractive

user=$1
full_name=$2
computer=$3
locale_name=$4
zone=$5
home=/home/$user

entry=$(getent passwd "$user" || true)
if [[ -n "$entry" && "$entry" != *',,,,Galaxy Terminal:'*":$home:/bin/bash" ]]; then
    printf 'GALAXY_ERROR=이미 사용 중인 Ubuntu 사용자 이름입니다.\n' >&2
    exit 1
fi

if ! command -v sudo >/dev/null || ! command -v locale-gen >/dev/null || [[ ! -f /usr/share/zoneinfo/$zone ]]; then
    printf 'GALAXY_STAGE=관리자 도구와 지역 데이터 설치 중\n'
    dpkg --configure -a </dev/null
    apt-get -o APT::Update::Error-Mode=any update </dev/null
    apt-get install -y --no-install-recommends sudo locales tzdata </dev/null
fi

printf 'GALAXY_STAGE=언어·지역과 시간대 설정 중\n'
if [[ $locale_name != C.UTF-8 ]]; then
    locale-gen "$locale_name" </dev/null
fi
update-locale --reset "LANG=$locale_name"
test -f "/usr/share/zoneinfo/$zone"
ln -sfn "/usr/share/zoneinfo/$zone" /etc/localtime
printf '%s\n' "$zone" >/etc/timezone
printf '%s\n' "$computer" >/etc/hostname
if ! grep -Fqx "127.0.1.1 $computer" /etc/hosts; then
    printf '\n127.0.1.1 %s\n' "$computer" >>/etc/hosts
fi

printf 'GALAXY_STAGE=Ubuntu 사용자 계정 생성 중\n'
if [[ -z "$entry" ]]; then
    useradd --create-home --user-group --shell /bin/bash --comment "$full_name,,,,Galaxy Terminal" "$user"
fi
usermod -aG sudo "$user"
touch /root/.hushlogin "$home/.hushlogin"
chown "$user:$(id -gn "$user")" "$home/.hushlogin"
chpasswd

# PRoot does not expose Ubuntu supplementary groups to sudo reliably.
policy=$(mktemp /etc/sudoers.d/.galaxy-account.XXXXXX)
trap 'rm -f "$policy"' EXIT
printf '%s ALL=(ALL:ALL) ALL\n' "$user" >"$policy"
chmod 0440 "$policy"
visudo -cf "$policy" >/dev/null
mv -f "$policy" "/etc/sudoers.d/90-galaxy-$user"
printf 'GALAXY_ACCOUNT=%s:%s\n' "$(id -u "$user")" "$(id -g "$user")"
