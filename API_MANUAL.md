# API Manual

## 1) Server Overview

- Go API Server
  - Version: `1.0.1` (`GET /version`)
  - Default Port: `9800`
- Callback Server
  - Version: `0.0.2`
  - Default Port: `9000`

권장 운영 구조:
- 외부 클라이언트 -> `go-api-server`는 호스트 IP:포트로 접근
- `go-api-server` -> `callback-server` 콜백은 동일 Docker user-defined network에서 컨테이너 이름 사용
  - 예: `http://callback-server:9000/download-notify`

---

## 2) Go API Server (`:9800`)

### 2.1 `GET /version`
- 목적: 서버 버전 확인
- 요청: 없음
- 성공 응답 예시:
```json
{
  "statusCode": 200,
  "version": "1.0.1"
}
```
- 테스트 케이스:
  - [정상] `curl -s http://localhost:9800/version`
  - [형식] `statusCode=200`, `version` 문자열 존재 확인

---

### 2.2 `POST /info`
- 목적: 대상 장치 `info.cgi` 조회
- 요청 바디:
```json
{
  "deviceIp": "10.47.61.87"
}
```
- 성공: key-value JSON 반환
- 실패: 공통 에러 형식
```json
{
  "statusCode": 502,
  "error": "..."
}
```
- 테스트 케이스:
  - [정상] 올바른 `deviceIp` 입력
  - [입력오류] `deviceIp` 누락 -> `400`
  - [통신오류] 존재하지 않는 IP -> `502` (타임아웃/연결 실패)

---

### 2.3 `POST /download`
- 목적: 대상 장치 `download.cgi`를 받아 컨테이너 파일 생성
- 요청 바디:
```json
{
  "requestId": 1001001,
  "deviceIp": "10.47.61.87",
  "rangeBegin": "20260322T184000",
  "rangeEnd": "20260322T184200",
  "targetFolder": "/data/download_result",
  "channelList": [
    { "channel": 1, "name": "cam01.avi" },
    { "channel": 2, "name": "cam02.avi" }
  ],
  "callbackUrl": "http://callback-server:9000/download-notify"
}
```
- 주요 검증:
  - `deviceIp`, `rangeBegin`, `rangeEnd`, `targetFolder` 필수
  - `channelList` 필수, `channel`은 `1..8`, 중복 불가
  - `name`은 파일명만 허용 (디렉터리 포함 불가)
  - Linux 실행 시 `targetFolder`는 절대 Linux 경로
- 성공 응답 예시:
```json
{
  "statusCode": 200,
  "message": "download completed",
  "requestId": 1001001,
  "saved": true,
  "targetPath": "/data/download_result",
  "startTime": "2026-03-22 18:40:00.000",
  "endTime": "2026-03-22 18:42:00.000",
  "videoFormat": "H264",
  "containerFps": 15,
  "containerList": [
    { "channel": 1, "path": "/data/download_result/cam01.avi" },
    { "channel": 2, "path": "/data/download_result/cam02.avi" }
  ],
  "containerFormat": "avi",
  "videoFrameCount": 1234
}
```
- 동시성 정책:
  - 같은 `deviceIp` 다운로드가 이미 진행 중이면 `409 Conflict`
  - 다른 `deviceIp`는 병렬 허용
- 테스트 케이스:
  - [정상] `callbackUrl` 포함하여 2채널 다운로드
  - [정상] `requestId` 미지정 시 자동 생성 확인
  - [입력오류] `channelList` 누락 -> `400`
  - [입력오류] `channelList.channel=9` -> `400`
  - [입력오류] Linux에서 상대 경로 `targetFolder` -> `400`
  - [중복실행] 동일 `deviceIp` 동시 2회 호출 -> 두 번째 `409`
  - [콜백] `running`, `failed`, `completed` 이벤트 수신 확인

---

### 2.4 `POST /record-list`
- 목적: 대상 장치 `datalist.cgi` 결과(현재 `drivingUnit`) 조회
- 요청:
```json
{
  "deviceIp": "10.47.61.87"
}
```
- 응답 예시:
```json
{
  "drivingUnit": {
    "count": 2,
    "items": [
      {
        "index": 0,
        "is_driving": 1,
        "channels": "1,2,3,4,5,6,7,8",
        "stime": "20260322T184000",
        "etime": "20260322T184200",
        "completed": false
      }
    ]
  },
  "statusCode": 200
}
```
- 변환 규칙:
  - `model_ch_flags` -> `channels`
  - 시간 `"yyyy/MM/dd HH:mm:ss"` -> `"yyyyMMddTHHmmss"`
- 테스트 케이스:
  - [정상] 유효 `deviceIp`
  - [입력오류] `deviceIp` 누락 -> `400`
  - [통신오류] 장치 미접속 -> `502`
  - [변환검증] `0x00ff` -> `"1,2,3,4,5,6,7,8"`

---

### 2.5 `GET /config`
- 목적: 통합 설정 조회
- 쿼리:
  - 전체: `/config`
  - 부분: `/config?option=connect|record|debug`
- 응답 예시(전체):
```json
{
  "connect": {
    "devicePort": 7000,
    "deviceUserId": "admin",
    "deviceUserPw": "000000"
  },
  "record": {
    "sourceFps": 7,
    "containerFormat": "avi",
    "containerOut": true
  },
  "debug": {
    "debug": true,
    "jpgOut": false
  },
  "statusCode": 200
}
```
- 테스트 케이스:
  - [정상] 전체 조회
  - [정상] `option=record` 부분 조회
  - [오류] `option=wrong` -> `400`

---

### 2.6 `PUT /config`
- 목적: 통합 설정 부분 업데이트
- 요청 예시:
```json
{
  "record": {
    "sourceFps": 15,
    "containerFormat": "mp4",
    "containerOut": true
  },
  "debug": {
    "debug": false
  }
}
```
- 검증:
  - `sourceFps` 1..120
  - `containerFormat`은 `avi|mp4`
  - `devicePort` 1..65535
- 테스트 케이스:
  - [정상] `record`만 수정
  - [정상] `connect + debug` 동시 수정
  - [오류] 빈 바디(모든 섹션 nil) -> `400`
  - [오류] `sourceFps=0` -> `400`
  - [오류] `containerFormat=mov` -> `400`

---

### 2.7 `GET /host-scan/config`
- 목적: host-scan용 접속 기본값 조회
- 테스트 케이스:
  - [정상] 반환 필드 `port`, `sourceFps`, `user`, `pw` 확인

### 2.8 `PUT /host-scan/config`
- 목적: host-scan용 접속 기본값 수정
- 요청 예시:
```json
{
  "port": 7000,
  "user": "admin",
  "pw": "000000"
}
```
- 테스트 케이스:
  - [정상] 유효 값 수정
  - [오류] `port=70000` -> `400`
  - [오류] `user` 또는 `pw` 공백 -> `400`

---

### 2.9 `GET /host-scan/scheduler`
- 목적: host-scan scheduler 상태 조회
- 응답: `enabled` true/false
- 테스트 케이스:
  - [정상] 초기 상태 확인

### 2.10 `PUT /host-scan/scheduler`
- 목적: scheduler on/off
- 요청:
```json
{
  "enabled": true
}
```
- 테스트 케이스:
  - [정상] `true` -> 켜짐 메시지 확인
  - [정상] `false` -> 꺼짐 메시지 확인

---

### 2.11 `GET /users`, `POST /users` (샘플 API)
- `GET /users`: 샘플 고정 유저 응답
- `POST /users`: 요청 body를 그대로 생성 응답
- 테스트 케이스:
  - [정상] GET 응답 확인
  - [정상] POST 유저 생성 형태 확인
  - [오류] POST 잘못된 JSON -> `400`

---

## 3) Callback Server (`:9000`)

### 3.1 `POST /download-notify`
- 목적: go-api-server가 전송하는 다운로드 이벤트 수신
- 요청(예시):
```json
{
  "event": "running",
  "requestId": 1001001,
  "deviceIp": "10.47.61.87",
  "goServerIp": "172.18.0.3",
  "message": "download in progress",
  "timestamp": "2026-03-23T03:30:00Z",
  "targetPath": "/data/download_result"
}
```
- 성공 응답:
```json
{
  "statusCode": 200,
  "message": "callback received"
}
```
- 저장 동작:
  - 메모리에 최근 200개 이벤트 유지
  - `receivedAt` 필드 서버에서 추가
- 테스트 케이스:
  - [정상] `running` 이벤트 수신 -> 200
  - [정상] `completed` 이벤트(`saved=true`) 수신
  - [오류] 필드 오타/불필요 필드 포함 JSON -> `400` (`DisallowUnknownFields`)

---

### 3.2 `GET /events`
- 목적: 수신 이벤트 조회
- 응답 예시:
```json
{
  "statusCode": 200,
  "count": 2,
  "events": [
    {
      "event": "running",
      "requestId": 1001001,
      "deviceIp": "10.47.61.87",
      "goServerIp": "172.18.0.3",
      "message": "download in progress",
      "timestamp": "2026-03-23T03:30:00Z",
      "targetPath": "/data/download_result",
      "receivedAt": "2026-03-23T03:30:02Z"
    }
  ]
}
```
- 테스트 케이스:
  - [정상] 빈 상태에서 `count=0`
  - [정상] 콜백 1회 후 `count=1`
  - [정상] 200건 초과 입력 시 최근 200건 유지

---

## 4) End-to-End Test Scenario (권장)

1. callback 서버 기동 후 `GET /events`로 빈 상태 확인  
2. go-api 서버 기동 후 `GET /version` 확인  
3. `POST /download` 호출 (`callbackUrl`은 내부 DNS 권장: `http://callback-server:9000/download-notify`)  
4. callback 로그(`docker logs -f callback-server`)와 `GET /events`로 이벤트 수신 검증  
5. `targetFolder` 결과 파일 존재 여부 확인  

---

## 5) Common Error Format

go-api-server 핸들러 공통 에러:
```json
{
  "statusCode": 400,
  "error": "message"
}
```

대표 코드:
- `400`: 요청 바디/파라미터 오류
- `409`: 동일 `deviceIp` 다운로드 중복
- `502`: 대상 장치/네트워크/CGI 연동 실패

