data "aws_a" "x" {
  zap = data.aws_b.y.id
}
data "aws_b" "y" {
  zap = data.aws_a.x.id
}
data "aws_leaf" "l" {
  most_recent = true
}
data "aws_mid1" "m1" {
  z = data.aws_leaf.l.id
}
data "aws_mid2" "m2" {
  z = data.aws_leaf.l.id
}
resource "aws_instance" "web" {
  ami      = data.aws_mid1.m1.id
  also_ami = data.aws_mid2.m2.id
}
