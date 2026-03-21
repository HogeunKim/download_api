# Ubuntu Docker 배포 프롬프트

## 목표
- Ubuntu에서 코드를 내려받아 Docker 이미지 빌드/실행/검증까지 완료한다.

## 입력값
- 저장소 URL: `https://github.com/HogeunKim/download_api.git`
- 컨테이너 이름: `go-api-server`
- 포트 매핑: `9800:9800`
- 호스트 폴더: `/home/genes007/download_api/download_result`
- 컨테이너 폴더: `/home/genes007/download_api/download_api`

## 요청 작업
1. Ubuntu에서 Docker/Git 설치 및 권한 설정 명령을 순서대로 제시한다.
2. 저장소 clone 후 이미지 빌드/컨테이너 실행 명령을 제시한다.
3. 볼륨 매핑 기준으로 `targetFolder`에 넣어야 할 정확한 값을 설명한다.
4. `docker inspect`, `docker logs`, `ls` 기반 검증 명령을 제시한다.
5. 자주 발생하는 권한 에러(`docker.sock`) 해결 절차를 제시한다.

## 기대 출력 형식
- 복붙 가능한 명령 블록
- 성공 기준 체크리스트
- 재배포(중지/삭제/재실행) 명령
