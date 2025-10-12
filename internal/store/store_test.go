package store

import (
	"reflect"
	"testing"

	vault "github.com/hashicorp/vault/api"
	bao "github.com/openbao/openbao/api/v2"
)

func Test_coerce(t *testing.T) {
	secret := "5ae27bf3-9ff4-474d-a538-c6bf3fccde6e"
	type args struct {
		s            any
		err          error
		fieldsFilter []string
	}
	tests := []struct {
		name    string
		args    args
		want    map[string]any
		wantErr bool
	}{
		{
			name: "bao",
			args: args{
				s: &bao.Secret{
					Data: map[string]any{
						"other":     "abc",
						"secret_id": secret,
					},
				},
				err:          nil,
				fieldsFilter: []string{"secret_id"},
			},
			wantErr: false,
			want: map[string]any{
				"secret_id": secret,
			},
		},
		{
			name: "vault",
			args: args{
				s: &vault.Secret{
					Data: map[string]any{
						"other":     "abc",
						"secret_id": secret,
					},
				},
				err:          nil,
				fieldsFilter: []string{"secret_id"},
			},
			wantErr: false,
			want: map[string]any{
				"secret_id": secret,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := coerce(tt.args.s, tt.args.err, tt.args.fieldsFilter...)
			if (err != nil) != tt.wantErr {
				t.Errorf("Coerce() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Coerce() got = %v, want %v", got, tt.want)
			}
		})
	}
}
