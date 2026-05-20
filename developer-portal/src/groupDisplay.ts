export function getGroupDisplayName(groupName: string): string {
  if (groupName === '__management__') {
    return 'Management'
  }
  return groupName
}
