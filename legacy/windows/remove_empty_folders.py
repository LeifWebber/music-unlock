import os
import shutil

def is_folder_empty_or_lrc_only(path):
    """
    检查文件夹是否为空或者只包含一个.lrc文件

    参数:
    path (str): 要检查的文件夹路径

    返回:
    bool: 如果文件夹为空或只有一个.lrc文件则返回True
    """
    items = os.listdir(path)

    # 如果文件夹为空，返回True
    if not items:
        return True

    # 如果只有一个文件且是.lrc文件，返回True
    if len(items) == 1 and items[0].lower().endswith('.lrc'):
        return True

    return False

def remove_empty_folders(path, remove_root=True):
    """
    递归删除指定路径下的所有空文件夹和只包含.lrc文件的文件夹

    参数:
    path (str): 要清理的目标文件夹路径
    remove_root (bool): 如果目标文件夹符合删除条件，是否删除目标文件夹本身

    返回:
    int: 删除的文件夹数量
    """
    # 确保路径存在
    if not os.path.isdir(path):
        print(f"错误: {path} 不是一个有效的目录")
        return 0

    removed_count = 0

    # 遍历所有文件夹（从底向上）
    for root, dirs, files in os.walk(path, topdown=False):
        # 检查每个子文件夹
        for dirname in dirs:
            full_path = os.path.join(root, dirname)
            try:
                # 检查文件夹是否为空或只有.lrc文件
                if is_folder_empty_or_lrc_only(full_path):
                    shutil.rmtree(full_path)
                    print(f"已删除文件夹: {full_path}")
                    removed_count += 1
            except OSError as e:
                print(f"删除 {full_path} 时发生错误: {e}")

    # 检查是否需要删除根目录
    if remove_root and is_folder_empty_or_lrc_only(path):
        try:
            shutil.rmtree(path)
            print(f"已删除根目录: {path}")
            removed_count += 1
        except OSError as e:
            print(f"删除根目录 {path} 时发生错误: {e}")

    return removed_count

# 使用示例
if __name__ == "__main__":
    import sys

    if len(sys.argv) < 2:
        print("使用方法: python script.py <目标文件夹路径> [--keep-root]")
        sys.exit(1)

    target_path = sys.argv[1]
    keep_root = "--keep-root" in sys.argv

    print(f"开始清理文件夹: {target_path}")
    print("将删除：1) 空文件夹 2) 只包含单个.lrc文件的文件夹")
    count = remove_empty_folders(target_path, not keep_root)
    print(f"清理完成，共删除 {count} 个文件夹")