
import os
import re

def is_broken(text):
    # Check for replacement character
    if '\ufffd' in text:
        return True
    # Check for common UTF-8 misinterpreted as Latin-1 patterns
    # e.g., "ì", "ë", "í" followed by other characters
    if re.search(r'[\u00cc\u00eb\u00ed][\u0080-\u00bf]', text):
        return True
    return False

def check_files(root_dir):
    broken_files = []
    for root, dirs, files in os.walk(root_dir):
        if '.git' in root or 'node_modules' in root:
            continue
        for file in files:
            if file.endswith(('.go', '.md', '.txt', '.json', '.sql')):
                path = os.path.join(root, file)
                try:
                    with open(path, 'r', encoding='utf-8') as f:
                        content = f.read()
                        if is_broken(content):
                            broken_files.append(path)
                except UnicodeDecodeError:
                    # If it's not UTF-8, it's already a candidate for "broken" in a modern environment
                    broken_files.append(path + " (NOT UTF-8)")
    return broken_files

if __name__ == "__main__":
    results = check_files('c:\\Dev\\Argus')
    for r in results:
        print(r)
