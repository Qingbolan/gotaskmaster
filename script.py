for i in range(10):
    print(i)
    
class Node:
    def __init__(self, value):
        self.value = value
        self.left = None
        self.right = None
    
    def is_same_banary_tree(self, other):
        def dfs(node1, node2):
            if not node1 and not node2:
                return True
            if not node1 or not node2:
                return False
            if node1.value != node2.value:
                return False
            return dfs(node1.left, node2.left) and dfs(node1.right, node2.right)
        return dfs(self, other)
    
    def add_left(self, value):
        self.left = Node(value)
        return self.left
    
    def add_right(self, value):
        self.right = Node(value)
        return self.right
    
root1 = Node(1)
root1.add_left(2).add_left(3)
root1.add_right(4).add_right(5)

root2 = Node(1)
root2.add_left(2).add_left(3)
root2.add_right(4).add_right(5)


root3 = Node(1)
root3.add_left(2).add_left(3)
root3.add_right(4).add_right(6)

if __name__ == '__main__':
    print(f"root1 is same as root2: {root1.is_same_banary_tree(root2)}")
    print(f"root1 is same as root3: {root1.is_same_banary_tree(root3)}")
    