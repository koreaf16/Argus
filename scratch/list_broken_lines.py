
import os
import re
import sys

# Ensure stdout uses utf-8
sys.stdout.reconfigure(encoding='utf-8')

def find_broken_lines(path):
    broken_lines = []
    try:
        with open(path, 'r', encoding='utf-8') as f:
            for i, line in enumerate(f, 1):
                if '??' in line or '\ufffd' in line:
                    broken_lines.append((i, line.strip()))
                elif re.search(r'[\x80-\xff]{2,}', line): # Search for non-ascii sequences
                    broken_lines.append((i, line.strip()))
    except Exception:
        pass
    return broken_lines

def main():
    root = 'c:\\Dev\\Argus\\infractl_참고용'
    output_file = 'c:\\Dev\\Argus\\scratch\\broken_lines_report.txt'
    with open(output_file, 'w', encoding='utf-8') as out:
        for r, d, files in os.walk(root):
            for file in files:
                if file.endswith('.go'):
                    path = os.path.join(r, file)
                    broken = find_broken_lines(path)
                    if broken:
                        out.write(f"FILE: {path}\n")
                        for line_no, content in broken:
                            out.write(f"  Line {line_no}: {content}\n")

if __name__ == "__main__":
    main()
