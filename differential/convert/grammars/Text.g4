// Text whose spaces are a token, so a node's tokens are contiguous in the
// source and GetText returns a slice of it, and whose entry rule goes on after
// EOF with an empty rule, so its EOF child is not its last child. Comments on
// the hidden channel and skipped newlines put gaps between tokens.
grammar Text;

prog : part+ EOF tail ;
tail : ;
part : WORD | NUM | SP | group ;
group : '(' part* ')' ;

WORD : [a-z]+ ;
NUM : [0-9]+ ;
SP : ' '+ ;
COMMENT : '#' ~[\n]* -> channel(HIDDEN) ;
NL : '\n' -> skip ;
