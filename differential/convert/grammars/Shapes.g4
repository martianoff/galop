// The shapes of left-recursive alternatives ANTLR's rewrite tells apart, each
// in a rule small enough that generated sentences reach every alternative:
// binary (left- and right-associative, with a middle part, with an operator
// rule), prefix (at high and at low precedence), suffix (with and without
// operands inside), primary, labeled and unlabeled alternatives, element and
// list labels, and left-recursive rules calling each other from a primary.
grammar Shapes;

prog : stat+ EOF ;

stat : e ';'          # exprStat
     | c ';'          # condStat
     | 'type' t ';'   # typeStat
     | 'inc' s ';'    # incStat
     ;

e : '-' e                                    # neg
  | e '!'                                    # fact
  | e '[' e ']'                              # index
  | e '(' (args+=e (',' args+=e)*)? ')'      # call
  | e '.' ID                                 # field
  | <assoc=right> left=e '^' right=e         # pow
  | e op=('*' | '/') e                       # mul
  | e op=('+' | '-') e                       # add
  | e cmp e                                  # compare
  | e '?' e ':' e                            # cond
  | 'not' e                                  # not
  | <assoc=right> e '=' e                    # assign
  | '(' e ')'                                # paren
  | ID                                       # id
  | INT                                      # int
  ;

cmp : '<' | '>' | '<=' ;

// Primaries that call another left-recursive rule.
c : c '&&' c
  | c '||' c
  | e '==' e
  | 'true'
  | '(' c ')'
  ;

// Unlabeled: a suffix and a binary alternative.
t : t '[' ']'
  | t '|' t
  | ID
  ;

// Suffixes only.
s : s '++'
  | s '--'
  | ID
  ;

ID : [a-z]+ ;
INT : [0-9]+ ;
WS : [ \t\r\n]+ -> skip ;
