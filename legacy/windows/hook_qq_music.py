import shutil
import frida
import os
import hashlib
import argparse
import yaml
from pathlib import Path

def read_config():
    """读取配置文件"""
    config_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'music.yaml')
    try:
        with open(config_path, 'r', encoding='utf-8') as f:
            config = yaml.safe_load(f)
            
        # 检查必要的配置项
        if not config.get('source_dir') or not config.get('target_dir'):
            print("配置文件格式错误！请确保 music.yaml 包含 source_dir 和 target_dir")
            print("示例配置：")
            print("source_dir: \"D:\\媒体\\音乐\"")
            print("target_dir: \"D:\\媒体\\新建文件夹\"")
            return None, None
            
        return config['source_dir'], config['target_dir']
    except FileNotFoundError:
        print("未找到配置文件 music.yaml！")
        print("请在程序根目录创建 music.yaml 文件，包含以下内容：")
        print("source_dir: \"D:\\媒体\\音乐\"")
        print("target_dir: \"D:\\媒体\\目标文件夹\"")
        return None, None
    except yaml.YAMLError:
        print("配置文件格式错误！请检查 music.yaml 的语法")
        return None, None

def move_files_flat(source_dir, target_dir):
    """直接移动所有文件到目标目录（不保持目录结构）"""
    print("开始移动文件...")
    
    # 遍历源目录下的所有文件和子目录
    for root, dirs, files in os.walk(source_dir):
        for file in files:
            # 跳过临时目录
            if root == "output":
                continue
                
            file_path = os.path.join(root, file)
            file_ext = os.path.splitext(file)[1].lower()
            
            # 处理音乐文件和歌词文件
            if file_ext in ['.mp3', '.flac', '.ogg', '.lrc']:
                # 直接移动到目标目录
                target_file = os.path.join(target_dir, file)
                
                # 如果目标文件已存在，添加数字后缀
                base, ext = os.path.splitext(target_file)
                counter = 1
                while os.path.exists(target_file):
                    target_file = f"{base}_{counter}{ext}"
                    counter += 1
                
                # 移动文件
                shutil.copy2(file_path, target_file)
                print(f"已移动: {file} -> {target_file}")
                
                # 删除源文件
                os.remove(file_path)
    
    # 从最深层目录开始删除空目录
    for root, dirs, files in os.walk(source_dir, topdown=False):
        try:
            if not os.listdir(root) and root != source_dir:
                os.rmdir(root)
                print(f"已删除空目录: {root}")
        except OSError:
            pass

def get_artist_folder(file_path):
    """从文件名中提取歌手名作为文件夹名"""
    # 假设文件名格式为: "歌名 - 歌手.扩展名"
    try:
        file_name = os.path.splitext(os.path.basename(file_path))[0]
        artist = file_name.split(" - ")[1].split("_")[0].strip()  # 处理可能包含多个歌手的情况
        return artist
    except:
        return "未分类"

def move_related_files(source_dir, target_dir):
    """移动所有音乐文件及其关联的歌词文件到目标目录（保持歌手分类）"""
    print("开始移动文件...")
    
    # 遍历源目录下的所有文件和子目录
    for root, dirs, files in os.walk(source_dir):
        for file in files:
            # 跳过临时目录
            if root == "output":
                continue
                
            file_path = os.path.join(root, file)
            file_ext = os.path.splitext(file)[1].lower()
            
            # 处理音乐文件和歌词文件
            if file_ext in ['.mp3', '.flac', '.ogg', '.lrc']:
                # 获取相对路径
                relative_path = os.path.relpath(root, source_dir)
                path_parts = relative_path.split(os.sep)
                
                # 确定歌手文件夹名
                if relative_path != "." and any(part.lower() != 'vipsongsdownload' for part in path_parts):
                    # 使用最后一个非特殊文件夹名作为歌手名
                    artist_folder = next((part for part in reversed(path_parts) 
                                       if part.lower() not in ['vipsongsdownload', 'output']), 
                                      get_artist_folder(file))
                else:
                    # 如果文件在根目录或只有特殊文件夹，从文件名提取歌手名
                    artist_folder = get_artist_folder(file)
                
                # 创建目标文件夹
                target_artist_dir = os.path.join(target_dir, artist_folder)
                os.makedirs(target_artist_dir, exist_ok=True)
                
                # 移动文件
                target_file = os.path.join(target_artist_dir, file)
                shutil.copy2(file_path, target_file)
                print(f"已移动: {file} -> {target_file}")
                
                # 删除源文件
                os.remove(file_path)
    
    # 从最深层目录开始删除空目录
    for root, dirs, files in os.walk(source_dir, topdown=False):
        try:
            if not os.listdir(root) and root != source_dir:
                os.rmdir(root)
                print(f"已删除空目录: {root}")
        except OSError:
            pass

def process_files(source_path, target_path=None, keep_structure=True):
    # 挂钩 QQ 音乐进程
    session = frida.attach("QQMusic.exe")

    # 加载并执行 JavaScript 脚本
    script = session.create_script(open("hook_qq_music.js", "r", encoding="utf-8").read())
    script.load()

    # 创建临时输出目录
    output_dir = "output"
    if not os.path.exists(output_dir):
        os.makedirs(output_dir)

    # 确保目标目录存在
    if target_path:
        os.makedirs(target_path, exist_ok=True)

    # 获取用户音乐目录路径
    home = os.path.abspath(source_path)

    # 遍历目录下的所有文件（包括子目录）
    for root, dirs, files in os.walk(home):
        for file in files:
            file_path = os.path.splitext(file)

            # 只处理 .mflac 和 .mgg 文件
            if file_path[-1] in [".mflac", ".mgg"]:
                print("正在解密", file)

                # 修改文件扩展名
                file_path = list(file_path)
                file_path[-1] = file_path[-1].replace("mflac", "flac").replace("mgg", "ogg")
                file_path_str = "".join(file_path)

                # 创建临时文件路径
                tmp_file_path = hashlib.md5(file.encode()).hexdigest()
                tmp_file_path = os.path.join(output_dir, tmp_file_path)
                tmp_file_path = os.path.abspath(tmp_file_path)

                # 调用脚本中的 decrypt 方法解密文件
                data = script.exports_sync.decrypt(os.path.join(root, file), tmp_file_path)

                # 解密后保存在原位置
                final_path = os.path.join(root, file_path_str)
                
                # 确保目标目录存在
                os.makedirs(os.path.dirname(final_path), exist_ok=True)
                
                # 使用shutil.move替代os.rename来支持跨驱动器移动
                shutil.move(tmp_file_path, final_path)

                # 删除原加密文件
                original_file_path = os.path.join(root, file)
                os.remove(original_file_path)

    print("解密完成")

    # 如果指定了目标路径，移动所有文件
    if target_path:
        if keep_structure:
            move_related_files(source_path, target_path)
        else:
            move_files_flat(source_path, target_path)
        print(f"所有文件已移动到目标路径：{target_path}")
        print("源文件和空目录已清理")
    else:
        print("所有文件已在原位置解密")

    # 清理临时目录
    if os.path.exists(output_dir):
        shutil.rmtree(output_dir)

    # 分离会话
    session.detach()

def main():
    parser = argparse.ArgumentParser(description='QQ音乐加密文件解密工具')
    parser.add_argument('source_path', nargs='?', help='加密文件的源路径')
    parser.add_argument('target_path', nargs='?', help='解密文件的目标路径（可选）')
    
    args = parser.parse_args()
    
    if args.source_path:
        # 使用命令行参数
        process_files(args.source_path, args.target_path, keep_structure=True)
    else:
        # 从配置文件读取路径
        source_dir, target_dir = read_config()
        if source_dir and target_dir:
            process_files(source_dir, target_dir, keep_structure=False)
        else:
            parser.print_help()

if __name__ == "__main__":
    main()