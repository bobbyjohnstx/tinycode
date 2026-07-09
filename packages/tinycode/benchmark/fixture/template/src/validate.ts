export interface UserInput {
  name: string
  age: number
}

export function processUser(input: UserInput): string {
  // No validation — task 3 adds it
  return `${input.name} is ${input.age} years old`
}
