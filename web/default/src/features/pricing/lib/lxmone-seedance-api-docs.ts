/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
export function lxmoneSeedanceRequest(model: string) {
  return {
    model,
    prompt: 'A blue ball on a white table, fixed camera.',
    seconds: model === 'seedance-2-fast' || model === 'seedance-2-mini' ? 5 : 4,
    resolution: '480p',
    aspect_ratio: '16:9',
  }
}

export function lxmoneSeedancePythonExample(model: string): string {
  return `import json, os, time
from pathlib import Path
import requests

base = os.environ["NEW_API_BASE_URL"].rstrip("/")
headers = {"Authorization": "Bearer " + os.environ["NEW_API_KEY"]}
payload = json.loads(${JSON.stringify(JSON.stringify(lxmoneSeedanceRequest(model)))})
response = requests.post(base + "/v1/videos", headers=headers, json=payload, timeout=120)
response.raise_for_status()
job = response.json()
task_id = job["id"]
Path("video-task.json").write_text(json.dumps(job))
print("Saved task:", task_id)
for _ in range(120):
    response = requests.get(base + "/v1/videos/" + task_id, headers=headers, timeout=60)
    response.raise_for_status()
    job = response.json()
    if job["status"] == "completed":
        with requests.get(base + "/v1/videos/" + task_id + "/content",
                          headers=headers, stream=True, timeout=300) as media:
            media.raise_for_status()
            with open("video.mp4", "wb") as output:
                for chunk in media.iter_content(1024 * 1024):
                    output.write(chunk)
        break
    if job["status"] in ("failed", "cancelled", "expired"):
        raise RuntimeError(job)
    if job["status"] not in ("queued", "in_progress"):
        raise RuntimeError(job)
    time.sleep(15)
else:
    raise TimeoutError("Query the saved task ID later; do not submit again.")`
}
