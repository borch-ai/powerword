import json
import subprocess
import os
import datetime

project_id = "PVT_kwDODaaa484BaOEO"
start_date_id = "PVTF_lADODaaa484BaOEOzhVHQt8"
target_date_id = "PVTF_lADODaaa484BaOEOzhVHQuA"

env = os.environ.copy()

res = subprocess.run(["gh", "project", "item-list", "3", "--owner", "borch-ai", "--format", "json"], env=env, capture_output=True, text=True, check=True)
data = json.loads(res.stdout)
items = data.get("items", [])

# Give some staggered dates
current_date = datetime.date(2026, 6, 1)

for i, item in enumerate(items):
    item_id = item['id']
    duration = 3 + (i % 4) # 3 to 6 days
    
    start_str = current_date.strftime("%Y-%m-%d")
    end_date = current_date + datetime.timedelta(days=duration)
    target_str = end_date.strftime("%Y-%m-%d")
    
    subprocess.run([
        "gh", "project", "item-edit",
        "--id", item_id,
        "--project-id", project_id,
        "--field-id", start_date_id,
        "--date", start_str
    ], env=env, check=True)
    
    subprocess.run([
        "gh", "project", "item-edit",
        "--id", item_id,
        "--project-id", project_id,
        "--field-id", target_date_id,
        "--date", target_str
    ], env=env, check=True)
    
    # stagger
    current_date = current_date + datetime.timedelta(days=(duration - 1))

print("Done assigning dates!")
