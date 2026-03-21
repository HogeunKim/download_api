# Download API 테스트 프롬프트

## 목표
- `/download` API를 현재 코드 기준으로 점검하고, 요청/응답/결과 파일 생성까지 확인한다.

## 입력값
- 서버 주소: `http://localhost:9800`
- 장비 IP: `10.47.61.87`
- 채널: `1,2,3,4`
- 시간 범위: `20260320T171020` ~ `20260320T171220`
- 컨테이너 내부 저장 경로: `/home/genes007/download_api/download_api`

## 요청 작업
1. 현재 `/config` 값을 조회하고 `record/debug` 값을 확인한다.
2. 필요하면 `PUT /config`로 `containerOut`, `containerFormat`, `debug`, `jpgOut`를 조정한다.
3. `/download` 요청용 curl 예시를 현재 스키마에 맞게 제시한다.
4. 결과 파일 확인 명령(컨테이너 내부/호스트)을 제시한다.
5. 파일 미생성 시 우선 점검 순서(마운트, targetFolder, 권한, 로그)를 제시한다.

## 기대 출력 형식
- 바로 실행 가능한 curl 명령
- 점검 명령 4~6개
- 실패 시 원인 우선순위 3개
